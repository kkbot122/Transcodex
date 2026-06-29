resource "aws_lb" "app" {
  name               = local.name
  load_balancer_type = "application"
  internal           = false
  security_groups    = [aws_security_group.alb.id]
  subnets            = [for subnet in aws_subnet.public : subnet.id]
}

resource "aws_lb_target_group" "api" {
  name        = "${local.name}-api"
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.main.id

  health_check {
    enabled             = true
    path                = "/healthz"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_target_group" "demo" {
  count = var.deploy_frontend_services ? 1 : 0

  name        = "${local.name}-demo"
  port        = 80
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.main.id

  health_check {
    enabled = true
    path    = "/"
    matcher = "200-399"
  }
}

resource "aws_lb_target_group" "dashboard" {
  count = var.deploy_frontend_services ? 1 : 0

  name        = "${local.name}-dashboard"
  port        = 80
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.main.id

  health_check {
    enabled = true
    path    = "/"
    matcher = "200-399"
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.app.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}

resource "aws_lb_listener_rule" "demo" {
  count = var.deploy_frontend_services ? 1 : 0

  listener_arn = aws_lb_listener.http.arn
  priority     = 100

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.demo[0].arn
  }

  condition {
    path_pattern {
      values = ["/demo/*"]
    }
  }
}

resource "aws_lb_listener_rule" "dashboard" {
  count = var.deploy_frontend_services ? 1 : 0

  listener_arn = aws_lb_listener.http.arn
  priority     = 110

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.dashboard[0].arn
  }

  condition {
    path_pattern {
      values = ["/dashboard/*"]
    }
  }
}

