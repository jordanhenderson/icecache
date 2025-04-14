// icycache.go (Dynamic Restore + Flush)
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fsnotify/fsnotify"
	"github.com/klauspost/compress/zstd"
)

var (
	s3Client       *s3.Client
	s3Bucket       = os.Getenv("S3_BUCKET")
	s3Prefix       = os.Getenv("S3_PREFIX")
	basePath       = "/tmp"
	restoreFileKey = fmt.Sprintf("%s/%s.zst", s3Prefix, os.Getenv("AWS_LAMBDA_FUNCTION_NAME"))

	encoder       *zstd.Encoder
	decoder       *zstd.Decoder
	stagedFiles   = make(map[string][]byte)
	mu            = sync.Mutex{}
	flushTimer    *time.Timer
	flushInterval = 1 * time.Second
)

func main() {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	s3Client = s3.NewFromConfig(cfg)

	encoder, err = zstd.NewWriter(nil)
	if err != nil {
		log.Fatalf("Failed to initialize ZSTD encoder: %v", err)
	}
	decoder, err = zstd.NewReader(nil)
	if err != nil {
		log.Fatalf("Failed to initialize ZSTD decoder: %v", err)
	}

	// Rehydrate
	err = rehydrateFromSingleFile(ctx, restoreFileKey)
	if err != nil {
		log.Printf("Warning: failed to rehydrate from file (%s): %v", restoreFileKey, err)
	} else {
		log.Printf("✅ Rehydrated state from s3://%s/%s", s3Bucket, restoreFileKey)
	}

	go watchAndCache(ctx)
	select {} // run forever
}

func rehydrateFromSingleFile(ctx context.Context, key string) error {
	resp, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3Bucket), Key: aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}
	defer resp.Body.Close()

	zreader, err := zstd.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to initialize ZSTD reader: %w", err)
	}
	defer zreader.Close()

	decoded, err := io.ReadAll(zreader)
	if err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}

	tarReader := tar.NewReader(bytes.NewReader(decoded))
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(basePath, hdr.Name)
		if hdr.Typeflag == tar.TypeDir {
			os.MkdirAll(target, fs.FileMode(hdr.Mode))
		} else {
			os.MkdirAll(filepath.Dir(target), 0755)
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			io.Copy(out, tarReader)
			out.Close()
		}
	}
	return nil
}

func watchAndCache(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Watcher init failed: %v", err)
	}
	defer watcher.Close()

	filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if d != nil && d.IsDir() {
			watcher.Add(path)
		}
		return nil
	})

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				cacheFile(event.Name)
				scheduleFlush(ctx)
			}
		case err := <-watcher.Errors:
			log.Println("Watcher error:", err)
		}
	}
}

func cacheFile(path string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}
	data, _ := os.ReadFile(path)
	mu.Lock()
	stagedFiles[path] = data
	mu.Unlock()
}

func scheduleFlush(ctx context.Context) {
	mu.Lock()
	defer mu.Unlock()
	if flushTimer != nil {
		flushTimer.Stop()
	}
	flushTimer = time.AfterFunc(flushInterval, func() {
		flushStagedToS3(ctx)
	})
}

func flushStagedToS3(ctx context.Context) {
	mu.Lock()
	defer mu.Unlock()
	if len(stagedFiles) == 0 {
		return
	}

	var buf bytes.Buffer
	tarWriter := tar.NewWriter(&buf)
	for path, data := range stagedFiles {
		rel, _ := filepath.Rel(basePath, path)
		tarWriter.WriteHeader(&tar.Header{Name: rel, Mode: 0644, Size: int64(len(data))})
		tarWriter.Write(data)
	}
	tarWriter.Close()

	compressed := encoder.EncodeAll(buf.Bytes(), nil)
	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3Bucket), Key: aws.String(restoreFileKey), Body: bytes.NewReader(compressed),
	})
	if err != nil {
		log.Printf("❌ Flush failed: %v", err)
	} else {
		log.Printf("✅ Flushed to s3://%s/%s", s3Bucket, restoreFileKey)
		stagedFiles = make(map[string][]byte)
	}
}
