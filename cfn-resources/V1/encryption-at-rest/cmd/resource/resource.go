package resource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/encryption-at-rest/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"go.mongodb.org/atlas/mongodbatlas"
)

// Create handles the Create event from the Cloudformation service.
func Create(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	encryptionAtRest := &mongodbatlas.EncryptionAtRest{
		AwsKms: mongodbatlas.AwsKms{
			Enabled:             currentModel.AwsKms.Enabled,
			AccessKeyID:         *currentModel.AwsKms.AccessKeyID,
			SecretAccessKey:     *currentModel.AwsKms.SecretAccessKey,
			CustomerMasterKeyID: *currentModel.AwsKms.CustomerMasterKeyID,
			Region:              *currentModel.AwsKms.Region,
		},
		GroupID: *currentModel.ProjectId,
	}

	_, _, err = client.EncryptionsAtRest.Create(context.Background(), encryptionAtRest)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating encryption at rest: %s", err)
	}

	cfnid := buildEncryptionAtRestCfnIdentifier(currentModel.ProjectId)
	currentModel.CfnPrimaryIdentifier = &cfnid
	// putting api keys and project name into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.CfnPrimaryIdentifier, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Create Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Read handles the Read event from the Cloudformation service.
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {

	params, err := getParameterFromParameterStore(currentModel.CfnPrimaryIdentifier, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	client, err := util.CreateMongoDBClient(*params.ApiKeys.PublicKey, *params.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	encryptionAtRest, _, err := client.EncryptionsAtRest.Get(context.Background(), *params.ProjectId)
	if err != nil {
		return handler.NewProgressEvent(), fmt.Errorf("error fetching encryption at rest configuration for project (%s): %s", *params.ProjectId, err)
	}

	currentModel.AwsKms.AccessKeyID = &encryptionAtRest.AwsKms.AccessKeyID
	currentModel.AwsKms.CustomerMasterKeyID = &encryptionAtRest.AwsKms.CustomerMasterKeyID
	currentModel.AwsKms.Enabled = encryptionAtRest.AwsKms.Enabled
	currentModel.AwsKms.Region = &encryptionAtRest.AwsKms.Region
	currentModel.AwsKms.SecretAccessKey = &encryptionAtRest.AwsKms.SecretAccessKey

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Read Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Update handles the Update event from the Cloudformation service.
func Update(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	// no-op
	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Update Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Delete handles the Delete event from the Cloudformation service.
func Delete(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	projectID := *currentModel.ProjectId

	_, err = client.EncryptionsAtRest.Delete(context.Background(), projectID)
	encryptionDeleted := true
	if err != nil {
		encryptionDeleted = false
	}

	_, err = deleteParameterFromParameterStore(currentModel.CfnPrimaryIdentifier, req.Session)
	parameterDeleted := true
	if err != nil {
		parameterDeleted = false
	}

	if !encryptionDeleted && !parameterDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete both encryption and parameter from store for encryption with id %s", *currentModel.CfnPrimaryIdentifier)
	}
	if !encryptionDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete encryption with id %s", *currentModel.CfnPrimaryIdentifier)
	}
	if !parameterDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete parameter for encrytption with id %s", *currentModel.CfnPrimaryIdentifier)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Delete Complete",
		ResourceModel:   currentModel,
	}, nil
}

// List handles the List event from the Cloudformation service.
func List(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	// no-op
	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "List Complete",
		ResourceModel:   currentModel,
	}, nil
}

type ParameterToBePersistedSpec struct {
	ApiKeys   *ApiKeyDefinition
	ProjectId *string
}

func putParameterIntoParameterStore(resourcePrimaryIdentifier *string, params *ParameterToBePersistedSpec, session *session.Session) (*ssm.PutParameterOutput, error) {
	ssmClient, err := util.CreateSSMClient(session)
	if err != nil {
		return nil, err
	}
	// transform api keys to json string
	parameterName := buildApiKeyParameterName(*resourcePrimaryIdentifier)
	byteParams, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	stringifiedParams := string(byteParams)
	parameterType := "SecureString"
	overwrite := true
	putParamOutput, err := ssmClient.PutParameter(&ssm.PutParameterInput{Name: &parameterName, Value: &stringifiedParams, Type: &parameterType, Overwrite: &overwrite})
	if err != nil {
		return nil, err
	}

	return putParamOutput, nil
}

func deleteParameterFromParameterStore(resourcePrimaryIdentifier *string, session *session.Session) (*ssm.DeleteParameterOutput, error) {
	ssmClient, err := util.CreateSSMClient(session)
	if err != nil {
		return nil, err
	}
	parameterName := buildApiKeyParameterName(*resourcePrimaryIdentifier)

	deleteParamOutput, err := ssmClient.DeleteParameter(&ssm.DeleteParameterInput{Name: &parameterName})
	if err != nil {
		return nil, err
	}

	return deleteParamOutput, nil
}

func getParameterFromParameterStore(resourcePrimaryIdentifier *string, session *session.Session) (*ParameterToBePersistedSpec, error) {
	ssmClient, err := util.CreateSSMClient(session)
	if err != nil {
		return nil, err
	}
	parameterName := buildApiKeyParameterName(*resourcePrimaryIdentifier)
	decrypt := true
	getParamOutput, err := ssmClient.GetParameter(&ssm.GetParameterInput{Name: &parameterName, WithDecryption: &decrypt})
	if err != nil {
		return nil, err
	}

	var params ParameterToBePersistedSpec
	err = json.Unmarshal([]byte(*getParamOutput.Parameter.Value), &params)
	if err != nil {
		return nil, err
	}
	return &params, nil
}

func buildApiKeyParameterName(resourcePrimaryIdentifier string) string {
	// this is strictly coupled with permissions for handlers, changing this means changing permissions in handler
	// moreover changing this might cause polution in parameter store -  be sure you know what you are doing
	parameterStorePrefix := "mongodbstpatlasv1encryptionatrest"
	return fmt.Sprintf("%s-%s", parameterStorePrefix, resourcePrimaryIdentifier)
}

func buildEncryptionAtRestCfnIdentifier(projectId *string) string {
	cfnid := fmt.Sprintf("%s-encryptionatrest", *projectId)
	return cfnid
}
