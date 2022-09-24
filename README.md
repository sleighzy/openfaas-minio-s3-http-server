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

## OpenFaaS and MinIO Credentials

To access files from the bucket the function requires credentials to
authenticate with MinIO. One option is to use a Service Account.

### MinIO Service Account

The MinIO user management [Create Service Accounts] documentation provides
details on creating a service account. This process will create the access key
and secret access key that can be provided as secrets to the OpenFaaS function.

The below IAM policy can be used to grant the service account access to the S3
bucket. This is named `website` in this example and must match the one specified
for the `S3_HTTP_BUCKET_NAME` environment variable of the function.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": ["arn:aws:s3:::website"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:ListMultipartUploadParts"],
      "Resource": ["arn:aws:s3:::website/*"]
    }
  ]
}
```

### OpenFaaS Secret Credentials

These secrets need to be added in the `openfaas-fn` namespace so that they are
available for use by the function. When the function is deployed the secrets
will be mounted as files to `/var/openfaas/secrets/website-access-key-id` and
`/var/openfaas/secrets/website-secret-access-key` and the values can be read by
the function. See [OpenFaas Using secrets] for more information.

```plain
kubectl create secret generic website-access-key-id \
  --from-literal website-access-key-id="GNxxxxxxx87" \
  --namespace openfaas-fn

kubectl create secret generic website-secret-access-key \
  --from-literal website-secret-access-key="9BTxxxxxxxxxxxxxxxxxxxD4T" \
  --namespace openfaas-fn
```

## Private Docker Registry

When deploying functions from a private registry OpenFaaS needs the credentials
to be able to authenticate to it when pulling images. See [Use a private
registry with Kubernetes] for more information on this.

Run the below command to create the Docker registry credentials secret in the
`openfaas-fn` namespace.

```plain
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

```plain
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

```plain
faas-cli template store pull golang-http
```

Run the command below to create the function definition files and empty function
handler.

```plain
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

The below commands were run to initialize the `go.mod` and `go.sum` files. These
commands need to be run from within the `minio-s3-http-server` directory
containing the function handler.

```plain
$ cd minio-s3-http-server
$ export GO111MODULE=on

$ go mod init
go: creating new go.mod: module openfaas/openfaas-minio-s3-http-server/minio-s3-http-server

$ go get
go: finding module for package github.com/openfaas/templates-sdk/go-http
go: found github.com/openfaas/templates-sdk/go-http in github.com/openfaas/templates-sdk v0.0.0-20200723110415-a699ec277c12

$ go mod tidy
```

When adding new libraries within your handler source code you will need to
update your Go dependencies.

```plain
cd minio-s3-http-server
go mod tidy
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

```plain
faas-cli build --shrinkwrap -f minio-s3-http-server.yml
```

### Docker Buildx for multiple platforms

The below commands should only need to be run once but will create a new Docker
build context for using with [Docker Buildx] to create images for multiple
platforms.

```plain
export DOCKER_CLI_EXPERIMENTAL=enabled
docker buildx create --use --name=multiarch
docker buildx inspect --bootstrap
```

Run the below command to use Buildx to create an image that supports both amd64
and arm64 architectures, and push it to the registry. This sets the
`GO111MODULE` build arg to `on` so that the Go dependencies are retrieved during
the build process. Whilst the `GO111MODULE` entry can be added to the
`minio-s3-http-server.yml` file as per the OpenFaaS documentation this does not appear to be
used when performing shrinkwrap builds, the argument must still be provided when
running `docker buildx build`.

```plain
$ docker buildx build \
 --build-arg GO111MODULE=on \
 --push \
 --tag registry.mydomain.io/openfaas/minio-s3-http-server:latest \
 --platform=linux/amd64,linux/arm64 \
 build/minio-s3-http-server/
```

## Deploying the Function

Run the below commands to point to the OpenFaaS gateway and authenticate.

```plain
$ export OPENFAAS_URL=https://gateway.mydomain.io
$ export PASSWORD=$(kubectl get secret -n openfaas basic-auth -o jsonpath="{.data.basic-auth-password}" | base64 --decode; echo)

$ echo -n $PASSWORD | faas-cli login --username admin --password-stdin
Calling the OpenFaaS server to validate the credentials...
credentials saved for admin https://gateway.mydomain.io
```

```plain
$ faas-cli deploy \
  --image registry.mydomain.io/openfaas/minio-s3-http-server:latest \
  --name minio-s3-http-server \
  --env S3_HTTP_DEBUG=true \
  --env S3_HTTP_LOG_LEVEL=debug \
  --env S3_HTTP_ENDPOINT=s3.mydomain.io \
  --env S3_HTTP_BUCKET_NAME=website \
  --env S3_HTTP_DEFAULT_PAGE=index.html \
  --secret website-access-key-id \
  --secret website-secret-access-key

Deployed. 202 Accepted.
URL: https://gateway.mydomain.io/function/minio-s3-http-server
```

## Serving Up Content

Create an S3 bucket named `website` as per the value of the
`S3_HTTP_BUCKET_NAME` environment variable above. Add an html page named
`index.html` with the below content into this bucket. The `./website/index.html`
file can be used for this example.

```html
<html>
  <body>
    <h1>Hello World!</h1>
  </body>
</html>
```

### Default page

The `S3_HTTP_DEFAULT_PAGE` environment variable can be used to specify the
default page that should be returned if one is not specified in the URL. This
defaults to `index.html` if the environment variable is not set.

## Invoking the Function

Using the awesome [HTTPie] command-line HTTP client, an excellent replacement
for `curl`, run the below command to invoke the function and request the
`index.html` page. This will retrieve the page from the S3 bucket and output its
contents.

```plain
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

```plain
faas-cli remove minio-s3-http-server
```

## License

[![MIT license]](https://lbesson.mit-license.org/)

[arkade]: https://github.com/alexellis/arkade
[create service accounts]:
  https://docs.min.io/minio/k8s/tutorials/user-management.html#create-service-accounts
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
