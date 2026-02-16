This repository contains my Go Gin REST API from Homework 1a, deployed and tested on AWS EC2 for Homework 1b.
The router.Run("0.0.0.0:8080") was initially router.Run("localhost:8080") before Homework1b


---

# CS6650 – Homework 5

## Product API with ECS Deployment and Load Testing

This project implements the **Product API** portion of the provided OpenAPI specification.
The service is written in Go using Gin, containerized with Docker, deployed to AWS ECS using Terraform, and stress tested using Locust.

---

# Project Structure

```
CS6650_2b_demo/
├── src/                # Go server code + Dockerfile
│   ├── main.go
│   ├── go.mod
│   ├── go.sum
│   ├── vendor/
│   └── Dockerfile
│
├── terraform/          # Infrastructure as Code (ECR, ECS, networking)
│
├── screenshots/        # Load test and deployment screenshots
│
└── README.md
```

* Server code: `src/main.go`
* Dockerfile: `src/Dockerfile`
* Infrastructure: `terraform/`

---

# How to Run Locally

Navigate to the `src` folder:

```
cd src
go mod tidy
go run main.go
```

Server will start at:

```
http://localhost:8080
```

---

# How to Run with Docker

From inside the `src` directory:

```
docker build -t product-api .
docker run -p 8080:8080 product-api
```

Then access:

```
http://localhost:8080
```

---

# How to Deploy to AWS Using Terraform

## Step 1: Configure AWS Credentials (Learner Lab)

Set your AWS credentials:

```
export AWS_ACCESS_KEY_ID="YOUR_KEY"
export AWS_SECRET_ACCESS_KEY="YOUR_SECRET"
export AWS_SESSION_TOKEN="YOUR_SESSION_TOKEN"
export AWS_DEFAULT_REGION="us-west-2"
```

Verify:

```
aws sts get-caller-identity
```

---

## Step 2: Deploy Infrastructure

Navigate to terraform folder:

```
cd terraform
terraform init
terraform plan
terraform apply
```

Terraform will:

* Build Docker image
* Push image to ECR
* Create ECS cluster
* Create ECS task definition
* Create ECS service (Fargate)
* Create security group allowing port 8080

After deployment:

1. Go to AWS ECS Console
2. Find running task
3. Open task details
4. Copy Public IP
5. Access service:

```
http://PUBLIC_IP:8080
```

---

# API Examples (Showing Required Response Codes)

## A) 200 OK – GET existing product

```
curl -i http://localhost:8080/products/1
```

---

## B) 404 Not Found – GET missing product

```
curl -i http://localhost:8080/products/999
```

---

## C) 204 No Content – POST valid product details

```
curl -i -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": 1,
    "sku": "ABC-123-XYZ",
    "manufacturer": "Acme Corporation",
    "category_id": 456,
    "weight": 1250,
    "some_other_id": 789
  }'
```

Expected response:

```
HTTP/1.1 204 No Content
```

---

## D) 400 Bad Request – Invalid input (mismatched id)

```
curl -i -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": 2,
    "sku": "ABC-123-XYZ",
    "manufacturer": "Acme Corporation",
    "category_id": 456,
    "weight": 1250,
    "some_other_id": 789
  }'
```

---

## E) 404 Not Found – POST product not found

```
curl -i -X POST http://localhost:8080/products/999/details \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": 999,
    "sku": "X",
    "manufacturer": "Y",
    "category_id": 1,
    "weight": 0,
    "some_other_id": 1
  }'
```

---

# Load Testing

Locust was used to stress test the deployed ECS service.

Test configurations:

* 10 users
* 50 users
* 100 users

Results:

* Throughput scaled almost linearly
* Approximately 63 requests per second at 100 users
* 0 percent failures
* Median response time remained around 60–65 ms
* Slight increase in 95th percentile under higher concurrency

Observations:

* GET and POST operations showed similar performance.
* Write lock overhead was minimal.
* Network latency dominated response time.
* ECS Fargate task was not saturated under tested load.

---

# Key Design Decisions

* Used map with RWMutex for O(1) lookup and safe concurrent access.
* Followed OpenAPI contract exactly for response codes.
* Used multi-stage Docker build for smaller production image.
* Used Terraform for declarative infrastructure deployment.
* Service is stateless and horizontally scalable.

---

# Notes

* Data is stored in memory and is not persistent.
* Scaling ECS to multiple tasks would require shared database.
* Terraform state files and credentials are excluded using `.gitignore`.

---
