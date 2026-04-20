# Distributed Ticket Reservation System

A distributed ticket booking backend for high-demand events, built to study how a reservation system can remain correct when many users try to reserve the same limited seats at the same time.

## Why I Built This

This project was built to explore one of the most important problems in distributed systems: **correctness under concurrency**.

In a flash-sale style event, many users may attempt to reserve the same small set of seats at nearly the same time. A ticketing backend must do more than respond quickly. It must also prevent:

- overselling
- double booking
- stale seat state across service instances
- broken reservation flows during scaling or failure

The goal of this project was to design and evaluate a backend that keeps reservation state correct under contention, then extend it into a cloud deployment with shared state, observability, scaling, and basic fault tolerance.

## Project Goals

The main goals of the project were:

- build a working end-to-end reservation flow
- prevent two users from successfully reserving the same seat
- support temporary holds and hold expiration
- run local load and contention experiments
- move from local in-memory state to shared cloud state
- deploy the service on AWS
- evaluate scaling and simple fault tolerance

## Core Reservation Flow

The system follows a simple seat lifecycle:

`available -> held -> reserved`

The booking flow is:

1. list available seats
2. place a temporary hold on a seat
3. confirm the reservation using the hold
4. release the seat if the hold expires

This design makes it possible to study reservation correctness directly.

## Architecture

### Local Prototype
- API Gateway
- Reservation Service
- Inventory access layer
- in-memory store for early development and testing

### Cloud Deployment
- Amazon ECS Fargate
- Application Load Balancer
- DynamoDB for shared seat, hold, and reservation state
- CloudWatch for logs and observability
- Terraform for infrastructure provisioning
- ECR for container image storage

### Final Cloud Request Path
`Locust -> ALB -> ECS Reservation Service -> DynamoDB -> CloudWatch`

## Key Features

- event and seat listing
- temporary seat holds
- reservation confirmation
- hold expiration and cleanup
- concurrency protection for same-seat requests
- shared DynamoDB backend for multi-instance deployment
- local and cloud load testing with Locust
- cloud deployment on AWS with ECS, ALB, and CloudWatch
- basic fault-tolerance test by stopping one ECS task during live traffic

## Experiments Performed

This project includes both local and cloud experiments.

### Local Experiments
- concurrent hold attempts on the same seat
- seat-state transition before and after reservation
- hold expiration validation
- local contention workload with Locust

### Cloud Experiments
- 1 ECS task behind ALB
- 2 ECS tasks behind ALB
- task-stop fault-tolerance experiment during active traffic

## Important Note About Reported Failures

Some Locust results show a high number of failed `POST /holds` requests.

These failures do **not** mean the backend was broken.

In most cases, they represent the expected business-rule response:

`seat is not available`

That means the system was correctly rejecting conflicting or invalid hold attempts instead of overselling seats.

## Project Progress Over Time

This repository reflects the project’s progression from initial design to final cloud deployment.

### Milestone 1
- created repo structure
- built API Gateway and Reservation Service
- defined Event, Seat, Hold, and Reservation models
- implemented local reservation flow
- added basic concurrency protection
- added Dockerfiles and README

### Milestone 2
- improved reservation correctness under concurrent access
- added hold expiration logic
- added stronger local tests
- created first Locust workloads
- ran baseline and contention experiments locally

### Milestone 3
- refactored storage layer for shared cloud state
- added DynamoDB backend
- deployed Reservation Service on AWS
- added ALB, ECS Fargate, ECR, CloudWatch, and Terraform
- ran cloud experiments with 1-task and 2-task deployments
- performed a task-stop fault-tolerance test

### Milestone 4
- organized final results
- prepared experiments report
- prepared presentation materials
- documented lessons learned

## Repository Activity

This repository is intended to show how the project evolved over time through:
- commits
- infrastructure changes
- code refactoring
- experiment scripts
- report and presentation preparation

The experiment code and supporting implementation used for the final project are included in this repository.

## Main Technologies

- Go
- Docker
- Python
- Locust
- Terraform
- AWS ECS Fargate
- AWS ALB
- AWS DynamoDB
- AWS CloudWatch
- AWS ECR

## How to Run Locally

### Run tests
```bash
go test ./...
````

### Run reservation service

```bash
go run ./cmd/reservation
```

### Run gateway

```bash
go run ./cmd/gateway
```

### Example local API checks

```bash
curl http://localhost:8080/health
curl http://localhost:8080/events
curl http://localhost:8080/events/evt-1/seats
```

## Cloud Notes

For cloud deployment, the project uses:

* Terraform to provision infrastructure
* ECR for image storage
* ECS Fargate for service deployment
* DynamoDB for shared state
* CloudWatch for service logs

The reservation service can switch between a memory backend for local work and a DynamoDB backend for AWS deployment.

## Author

**Jubril Akanbi**

## Final Summary

This project began as a local correctness-focused reservation prototype and evolved into a cloud-backed distributed system with shared state, observability, scaling, and fault-tolerance evaluation.

The main result is that the system preserves reservation correctness by rejecting conflicting seat requests rather than overselling inventory.