resource "aws_ecr_repository" "app" {
  for_each = local.ecr_repositories

  name                 = "${local.name}-${each.key}"
  image_tag_mutability = "MUTABLE"
  force_delete         = true

  image_scanning_configuration {
    scan_on_push = true
  }
}
