# icecache

**icecache** is a plug-and-play ephemeral state cache for AWS Lambda. It automatically restores and persists the contents of `/tmp` using compressed tar archives stored in S3. This allows Lambda functions to retain session state, cache files, or persist local databases across cold starts without any manual sync logic.

---

## 🧰 Features

- 🧊 Cold-start restoration from S3
- 🔄 Real-time change detection with `fsnotify`
- 💨 Debounced flush to avoid excessive writes
- 📦 Compact storage using `.tar.zst`
- 📁 File-based, no manifest required

---

## 🚀 Getting Started

### 1. Add the Layer

Include the `icecache` Lambda Layer to your function:

```
arn:aws:lambda:ap-southeast-2:503687392860:layer:icecache:1
```

Via AWS Console or CLI:

```bash
aws lambda update-function-configuration \
  --function-name YOUR_FUNCTION_NAME \
  --layers arn:aws:lambda:ap-southeast-2:503687392860:layer:icecache:1
```

---

### 2. Set the Exec Wrapper

Set the Lambda environment variable:

```bash
AWS_LAMBDA_EXEC_WRAPPER=/opt/icecache
```

This tells AWS Lambda to run `icecache` before your handler, automatically managing state restoration and sync.

---

### 3. Set Required Environment Variables

```bash
S3_BUCKET=your-bucket-name
S3_PREFIX=lambda-cache
```

---

### 4. IAM Permissions

Ensure your Lambda function has the following IAM permissions:

```json
{
  "Effect": "Allow",
  "Action": [
    "s3:GetObject",
    "s3:PutObject"
  ],
  "Resource": "arn:aws:s3:::your-bucket-name/lambda-cache/*"
}
```

---

## 🧊 How It Works

On cold start:
- Downloads `s3://$S3_BUCKET/$S3_PREFIX/$AWS_LAMBDA_FUNCTION_NAME.zst`
- Decompresses and extracts it into `/tmp`

During execution:
- Watches `/tmp` for changes
- Debounces for 1 second
- Flushes all modified files as a `.tar.zst` back to the same S3 key

---

## 💡 Use Cases

- Lightweight local SQLite or MySQL databases
- Retaining temporary render artifacts
- Persistent scratch space for AI model layers or unpacked binaries

---

## 📁 Output Format

- Stored file: `s3://<S3_BUCKET>/<S3_PREFIX>/<FUNCTION_NAME>.zst`
- Internally: `.tar` archive compressed with Zstandard

---

## 📦 Future Features

- Optional Zstandard dictionary support
- Background streaming flush
- Append-only archive logging
- Multi-version state snapshots

---

Need Terraform, SAM/CDK modules, or CI setup? Let me know @ jordan-henderson@outlook.com

