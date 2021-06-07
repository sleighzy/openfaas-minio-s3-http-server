package function

import (
	"io/ioutil"
	"net/http"

	handler "github.com/openfaas/templates-sdk/go-http"

	minio "github.com/minio/minio-go"

	log "github.com/sirupsen/logrus"

	envconfig "github.com/kelseyhightower/envconfig"
)

type Configuration struct {
	Debug           bool   `default:"false"`
	LogLevel        string `default:"info" split_words:"true"`
	Endpoint        string `required:"true"`
	BucketName      string `required:"true" split_words:"true"`
	AccessKeyId     string `required:"true" split_words:"true"`
	SecretAccessKey string `required:"true" split_words:"true"`
	UseSSL          bool   `default:"true" split_words:"true"`
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

	minioClient, err := minio.New(config.Endpoint, config.AccessKeyId, config.SecretAccessKey, config.UseSSL)
	if err != nil {
		log.Fatalln(err)
	}

	log.Infof("%#v\n", minioClient)

	objectName := req.QueryString
	log.Debugf("Requested page: '%s'", objectName)

	object, err := minioClient.GetObject(config.BucketName, objectName, minio.GetObjectOptions{})
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
