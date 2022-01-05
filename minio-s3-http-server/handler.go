package function

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"

	handler "github.com/openfaas/templates-sdk/go-http"

	minio "github.com/minio/minio-go/v7"

	credentials "github.com/minio/minio-go/v7/pkg/credentials"

	log "github.com/sirupsen/logrus"

	envconfig "github.com/kelseyhightower/envconfig"
)

type Configuration struct {
	Debug       bool   `default:"false"`
	LogLevel    string `default:"info" split_words:"true"`
	Endpoint    string `required:"true"`
	BucketName  string `required:"true" split_words:"true"`
	UseSSL      bool   `default:"true" split_words:"true"`
	DefaultPage string `default:"index.html" split_words:"true"`
}

// Handle a function invocation
func Handle(req handler.Request) (handler.Response, error) {
	var config Configuration
	err := envconfig.Process("S3_HTTP", &config)
	if err != nil {
		log.Fatalln(err)
	}

	switch config.LogLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	var accessKeyIdBytes []byte
	accessKeyIdBytes, err = ioutil.ReadFile("/var/openfaas/secrets/website-access-key-id")
	if err != nil {
		log.Fatal(err)
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
		}, err
	}

	accessKeyId := strings.TrimSpace(string(accessKeyIdBytes))
	if len(accessKeyId) == 0 {
		log.Fatal("Missing access key id")
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	var secretAccessKeyBytes []byte
	secretAccessKeyBytes, err = ioutil.ReadFile("/var/openfaas/secrets/website-secret-access-key")
	if err != nil {
		log.Fatal(err)
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
		}, err
	}

	secretAccessKey := strings.TrimSpace(string(secretAccessKeyBytes))
	if len(secretAccessKey) == 0 {
		log.Fatal("Missing secret access key")
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	minioClient, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyId, secretAccessKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		log.Fatalln(err)
	}

	log.Infof("%#v\n", minioClient)

	objectName := req.QueryString
	if objectName == "" {
		objectName = config.DefaultPage
		log.Debugf("No page requested, using default page: '%s'", objectName)
	} else {
		log.Debugf("Requested page: '%s'", objectName)
	}

	object, err := minioClient.GetObject(context.Background(), config.BucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		log.Errorln(err)
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
		}, err
	}
	defer object.Close()

	content, err := ioutil.ReadAll(object)
	if err != nil {
		log.Errorln(err)
		errResponse := minio.ToErrorResponse(err)
		switch errResponse.Code {
		case "AccessDenied":
			return handler.Response{
				StatusCode: http.StatusUnauthorized,
			}, nil
		case "NoSuchBucket", "InvalidBucketName", "NoSuchKey":
			return handler.Response{
				StatusCode: http.StatusNotFound,
			}, nil
		default:
			return handler.Response{
				StatusCode: http.StatusInternalServerError,
			}, err
		}
	}

	return handler.Response{
		Body:       content,
		StatusCode: http.StatusOK,
	}, err
}
