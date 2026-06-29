resource "aws_s3_bucket" "storage" {
  bucket_prefix = "${local.name}-"

  force_destroy = var.environment == "demo"
}

resource "aws_s3_bucket_public_access_block" "storage" {
  bucket = aws_s3_bucket.storage.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "storage" {
  bucket = aws_s3_bucket.storage.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_versioning" "storage" {
  bucket = aws_s3_bucket.storage.id

  versioning_configuration {
    status = "Enabled"
  }
}

data "aws_iam_policy_document" "storage_bucket" {
  statement {
    sid    = "AllowCloudFrontReadOnlyApprovedPrefixes"
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["cloudfront.amazonaws.com"]
    }

    actions = ["s3:GetObject"]

    resources = concat(
      ["${aws_s3_bucket.storage.arn}/outputs/*"],
      [for prefix in var.frontend_read_prefixes : "${aws_s3_bucket.storage.arn}/${prefix}"],
    )

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceArn"
      values   = [aws_cloudfront_distribution.main.arn]
    }
  }
}

resource "aws_s3_bucket_policy" "storage" {
  bucket = aws_s3_bucket.storage.id
  policy = data.aws_iam_policy_document.storage_bucket.json
}

