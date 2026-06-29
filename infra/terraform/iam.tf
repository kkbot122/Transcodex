data "aws_iam_policy_document" "ecs_task_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ecs_task_execution" {
  name               = "${local.name}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json
}

resource "aws_iam_role_policy_attachment" "ecs_task_execution" {
  role       = aws_iam_role.ecs_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "ecs_task_execution_secrets" {
  statement {
    effect = "Allow"
    actions = [
      "ssm:GetParameters",
      "ssm:GetParameter",
      "secretsmanager:GetSecretValue",
      "kms:Decrypt",
    ]
    resources = length(var.app_secret_arns) + length(var.frontend_secret_arns) > 0 ? concat(values(var.app_secret_arns), values(var.frontend_secret_arns)) : ["*"]
  }
}

resource "aws_iam_role_policy" "ecs_task_execution_secrets" {
  count = length(var.app_secret_arns) + length(var.frontend_secret_arns) > 0 ? 1 : 0

  name   = "${local.name}-ecs-secrets"
  role   = aws_iam_role.ecs_task_execution.id
  policy = data.aws_iam_policy_document.ecs_task_execution_secrets.json
}

resource "aws_iam_role" "ecs_app_task" {
  name               = "${local.name}-app-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json
}

data "aws_iam_policy_document" "ecs_app_task" {
  statement {
    sid    = "ObjectStorageAccess"
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
      "s3:AbortMultipartUpload",
    ]
    resources = [
      "${aws_s3_bucket.storage.arn}/raw/*",
      "${aws_s3_bucket.storage.arn}/outputs/*",
    ]
  }

  statement {
    sid    = "BucketReadAccess"
    effect = "Allow"
    actions = [
      "s3:GetBucketLocation",
      "s3:ListBucket",
    ]
    resources = [aws_s3_bucket.storage.arn]
  }
}

resource "aws_iam_role_policy" "ecs_app_task" {
  name   = "${local.name}-app-task"
  role   = aws_iam_role.ecs_app_task.id
  policy = data.aws_iam_policy_document.ecs_app_task.json
}
