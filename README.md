# OpenFaaS MinIO S3 HTTP Server Function

[MinIO] is an open source object storage server with support for the S3 API.
This means that you can run your very own S3 deployment from your homelab.

This OpenFaas function when invoked will retrieve an object from S3 and stream
back as HTTP. This can be used to host a website and serve up pages stored in an
S3 bucket.

## Installing OpenFaaS

I'm not going to go into any great detail for installing and deploying OpenFaaS,
I'll do that as a separate set of instructions later on. I essentially followed
the directions from [OpenFaas Deployment] and used the awesome [Arkade] CLI
installer for Kubernetes applications, plus some of the linked blog posts.

## Private Docker Registry

When deploying functions from a private registry OpenFaaS needs the credentials
to be able to authenticate to it when pulling images. See [Use a private
registry with Kubernetes] for more information on this.

Run the below command to create the Docker registry credentials secret in the
`openfaas-fn` namespace.

```sh
kubectl create secret docker-registry homelab-docker-registry \
  --docker-username=homelab-user \
  --docker-password=homelab-password \
  --docker-email=homelab@example.com \
  --docker-server=https://registry.mydomain.io \
  --namespace openfaas-fn
```

Add the below yaml to the `default` service account in the `openfaas-fn`
namespace so that it has the credentials to authenticate with the registry when
pulling images.

```sh
kubectl edit serviceaccount default -n openfaas-fn
```

```yaml
imagePullSecrets:
  - name: homelab-docker-registry
```

## Creating the Function

The below steps were followed to create a new function and handler.

Run the command below to pull the `golang-http` template that creates an HTTP
request handler for Golang.

```sh
faas-cli template store pull golang-http
```

Run the command below to create the function definition files and empty function
handler.

```sh
$ faas new --lang golang-http minio-s3-http-server
Folder: minio-s3-http-server created.
  ___                   _____           ____
 / _ \ _ __   ___ _ __ |  ___|_ _  __ _/ ___|
| | | | '_ \ / _ \ '_ \| |_ / _` |/ _` \___ \
| |_| | |_) |  __/ | | |  _| (_| | (_| |___) |
 \___/| .__/ \___|_| |_|_|  \__,_|\__,_|____/
      |_|


Function created in folder: minio-s3-http-server
Stack file written: minio-s3-http-server.yml
```

### Golang Dependencies

This function uses additional Go libraries that need to be included as
dependencies when building. See [GO - Dependencies] for options on including
these dependencies. This repository uses [Go Modules] for managing dependencies.

The below commands were run to initialize the `go.mod` and `go.sum` files, and
the contents of the `go.mod` file put in the `GO_REPLACE.txt` file to be used
during the build. These commands need to be run from within the
`minio-s3-http-server` directory containing the function handler.

```sh
$ cd minio-s3-http-server
$ export GO111MODULE=on

$ go mod init
go: creating new go.mod: module openfaas/openfaas-minio-s3-http-server/minio-s3-http-server

$ go get
go: finding module for package github.com/openfaas/templates-sdk/go-http
go: found github.com/openfaas/templates-sdk/go-http in github.com/openfaas/templates-sdk v0.0.0-20200723110415-a699ec277c12

$ go mod tidy
$ cat go.mod > GO_REPLACE.txt
```

When adding new libraries within your handler source code you will need to
update your Go dependencies.

```sh
cd minio-s3-http-server
go mod tidy
cat go.mod > GO_REPLACE.txt
```

## Building the Function

The OpenFaaS documentation and [Simple Serverless with Golang Functions and
Microservices] provide instruction on how to develop and build OpenFaaS
functions.

### ARM64 Image Builds

This function is going to be deployed onto a Raspberry Pi using ARM64 so the
build and deploy process is slightly different than a basic `faas-cli up`
command. The below command will create a new directory containing the
`Dockerfile` and artifacts that will be used to build the container image.

```sh
faas-cli build --shrinkwrap -f minio-s3-http-server.yml
```

### Docker Buildx for multiple platforms

The below commands should only need to be run once but will create a new Docker
build context for using with [Docker Buildx] to create images for multiple
platforms.

```sh
export DOCKER_CLI_EXPERIMENTAL=enabled
docker buildx create --use --name=multiarch
docker buildx inspect --bootstrap
```

Run the below command to use Buildx to create an image that supports both amd64
and arm64 architectures, and push it to the registry. This sets the
`GO111MODULE` build arg to `on` so that the `GO_REPLACE.txt` is used and the Go
dependencies retrieved during the build process. Whilst the `GO111MODULE` entry
can be added to the `slack.yml` file as per the OpenFaaS documentation this does
not appear to be used when performing shrinkwrap builds, the argument must still
be provided when running `docker buildx build`.

```sh
$ docker buildx build \
 --build-arg GO111MODULE=on \
 --push \
 --tag registry.mydomain.io/openfaas/minio-s3-http-server:latest \
 --platform=linux/amd64,linux/arm64 \
 build/minio-s3-http-server/
```

## Deploying the Function

Run the below commands to point to the OpenFaaS gateway and authenticate.

```sh
$ export OPENFAAS_URL=https://gateway.mydomain.io
$ export PASSWORD=$(kubectl get secret -n openfaas basic-auth -o jsonpath="{.data.basic-auth-password}" | base64 --decode; echo)

$ echo -n $PASSWORD | faas-cli login --username admin --password-stdin
Calling the OpenFaaS server to validate the credentials...
credentials saved for admin https://gateway.mydomain.io
```

```sh
$ faas-cli deploy \
  --image registry.mydomain.io/openfaas/minio-s3-http-server:0.1.0 \
  --name minio-s3-http-server \
  --env S3_HTTP_DEBUG=true \
  --env S3_HTTP_LOG_LEVEL=debug \
  --env S3_HTTP_ENDPOINT=s3.mydomain.io \
  --env S3_HTTP_BUCKET_NAME=website \
  --env S3_HTTP_ACCESS_KEY_ID=AKIYYYXXZZ7XXXZZ \
  --env S3_HTTP_SECRET_ACCESS_KEY=wXXzzWWI/K7XXHM/bPxRfiCYDEXXQQQ

Deployed. 202 Accepted.
URL: https://gateway.mydomain.io/function/minio-s3-http-server
```

## Serving Up Content

Create an S3 bucket named `website` as per the value of the
`S3_HTTP_BUCKET_NAME` environment variable above. Add an html page named
`index.html` with the below content into this bucket.

```html
<html>
  <body>
    <h1>Hello World!</h1>
  </body>
</html>
```

Using the awesome [HTTPie] command-line HTTP client, an excellent replacement
for `curl`, run the below command to invoke the function and request the
`index.html` page. This will retrieve the page from the S3 bucket and output its
contents.

```sh
$ http https://gateway.mydomain.io/function/minio-s3-http-server?index.html
HTTP/1.1 200 OK
Content-Length: 60
Content-Type: text/html; charset=utf-8
Date: Mon, 07 Jun 2021 08:38:05 GMT
X-Call-Id: 6f9b6d4e-4876-4c0b-8cf7-0692db76b338
X-Duration-Seconds: 0.236678
X-Start-Time: 1623055084985714841

<html>
  <body>
    <h1>Hello World!</h1>
  </body>
</html>
```

## Removing the Function

Run the below command to remove the function when it is no longer required.

```sh
faas-cli remove minio-s3-http-server
```

## License

[![MIT license]](https://lbesson.mit-license.org/)

[arkade]: https://github.com/alexellis/arkade
[docker buildx]:
  https://docs.docker.com/engine/reference/commandline/buildx_build/
[go - dependencies]: https://docs.openfaas.com/cli/templates/#go-go-dependencies
[go modules]: https://golang.org/ref/mod
[httpie]: https://httpie.io/
[minio]: https://min.io/
[mit license]: https://img.shields.io/badge/License-MIT-blue.svg
[openfaas]: https://www.openfaas.com/
[openfaas deployment]: https://docs.openfaas.com/deployment/
[openfaas using secrets]: https://docs.openfaas.com/reference/secrets/
[simple serverless with golang functions and microservices]:
  https://www.openfaas.com/blog/golang-serverless/
[use a private registry with kubernetes]:
  https://docs.openfaas.com/deployment/kubernetes/#use-a-private-registry-with-kubernetes
