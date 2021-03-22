package util

import (
	"github.com/Sectorbob/mlab-ns2/gae/ns/digest"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"go.mongodb.org/atlas/mongodbatlas"
)

const (
	Version = "beta"
)

func CreateMongoDBClient(publicKey, privateKey string) (*mongodbatlas.Client, error) {
	// setup a transport to handle digest
	transport := digest.NewTransport(publicKey, privateKey)

	// initialize the client
	client, err := transport.Client()
	if err != nil {
		return nil, err
	}

	//Initialize the MongoDB Atlas API Client.
	atlas := mongodbatlas.NewClient(client)
	atlas.UserAgent = "mongodbatlas-cloudformation-resources/" + Version
	return atlas, nil
}

func CreateSSMClient(session *session.Session) (*ssm.SSM, error) {
	ssmCli := ssm.New(session)
	return ssmCli, nil
}
