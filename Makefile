.PHONY: run build test docker-build docker-run clean deploy cloud-build

run:
	go run ./cmd/bird

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bird ./cmd/bird

test:
	go test -race -count=1 ./...

docker-build:
	docker build -f deploy/Dockerfile -t vostros .

docker-run:
	docker run -p 8080:8080 -e JWT_SECRET=dev -e DATABASE_URL=$(DATABASE_URL) vostros

cloud-build:
	gcloud builds submit --config=deploy/cloudbuild.yaml --substitutions=SHORT_SHA=$$(git rev-parse --short HEAD) .

deploy:
	gcloud run deploy vostros \
		--image us-central1-docker.pkg.dev/old-school-bird/bird/vostros:latest \
		--region us-central1 \
		--platform managed \
		--allow-unauthenticated \
		--min-instances 0 \
		--max-instances 10 \
		--memory 256Mi \
		--cpu 1 \
		--port 8080

clean:
	rm -rf bin/
