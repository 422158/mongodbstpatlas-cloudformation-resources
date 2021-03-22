package resource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/network-container/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	matlasClient "go.mongodb.org/atlas/mongodbatlas"
)

const (
	defaultProviderName = "AWS"
)

// Create handles the Create event from the Cloudformation service.
func Create(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	projectID := currentModel.ProjectId
	containerRequest := &matlasClient.Container{}
	containerRequest.RegionName = *currentModel.RegionName
	containerRequest.ProviderName = *currentModel.ProviderName
	containerRequest.AtlasCIDRBlock = *currentModel.AtlasCidrBlock
	containerResponse, _, err := client.Containers.Create(context.Background(), *projectID, containerRequest)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating network container: %s", err)
	}

	currentModel.Id = &containerResponse.ID
	currentModel.VpcId = &containerResponse.VPCID
	currentModel.Provisioned = containerResponse.Provisioned
	currentModel.AtlasCidrBlock = &containerResponse.AtlasCIDRBlock

	// putting api keys and project name into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.Id, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Create complete",
		ResourceModel:   currentModel,
	}, nil
}

// Read handles the Read event from the Cloudformation service.
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {

	params, err := getParameterFromParameterStore(currentModel.Id, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, err
	}
	client, err := util.CreateMongoDBClient(*params.ApiKeys.PublicKey, *params.ApiKeys.PrivateKey)

	if err != nil {
		return handler.ProgressEvent{}, err
	}

	containerID := *currentModel.Id

	containerResponse, _, err := client.Containers.Get(context.Background(), *params.ProjectId, containerID)

	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error reading container with id(project: %s, container: %s): %s", *params.ProjectId, containerID, err)
	}

	currentModel.RegionName = &containerResponse.RegionName
	currentModel.Provisioned = containerResponse.Provisioned
	currentModel.VpcId = &containerResponse.VPCID
	currentModel.AtlasCidrBlock = &containerResponse.AtlasCIDRBlock

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Read Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Update handles the Update event from the Cloudformation service.
func Update(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)

	if err != nil {
		return handler.ProgressEvent{}, err
	}

	projectID := *currentModel.ProjectId
	containerID := *currentModel.Id
	containerRequest := &matlasClient.Container{}
	providerName := currentModel.ProviderName
	if providerName == nil || *providerName == "" {
		aws := defaultProviderName
		providerName = &aws
	}
	CIDR := currentModel.AtlasCidrBlock
	if CIDR != nil {
		containerRequest.AtlasCIDRBlock = *CIDR
	}
	containerRequest.ProviderName = *providerName
	containerRequest.RegionName = *currentModel.RegionName
	containerResponse, _, err := client.Containers.Update(context.Background(), projectID, containerID, containerRequest)
	if err != nil {
		formattedContainerRequest, _ := json.Marshal(&containerRequest)
		return handler.ProgressEvent{}, fmt.Errorf("error updating container with id(project: %s, container: %s): %s", projectID, string(formattedContainerRequest), err)
	}

	currentModel.Id = &containerResponse.ID

	// putting api keys into parameter store (needed for read operation)
	// the api keys might have been updated therefore we need to do this here
	_, err = putParameterIntoParameterStore(currentModel.Id, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId}, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error when putting api keys into parameter store: %s", err)
	}

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
	containerID := *currentModel.Id

	_, err = client.Containers.Delete(context.Background(), projectID, containerID)
	containerDeleted := true
	if err != nil {
		containerDeleted = false
	}

	_, err = deleteParameterFromParameterStore(currentModel.Id, req.Session)
	parameterDeleted := true
	if err != nil {
		parameterDeleted = false
	}

	if !containerDeleted && !parameterDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete both container with id %s and parameter from store.", *currentModel.Id)
	}
	if !containerDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete container with id %s", *currentModel.Id)
	}
	if !parameterDeleted {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Failed to delete parameter for container with id %s", *currentModel.Id)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Delete Complete",
		ResourceModel:   currentModel,
	}, nil
}

// List handles the List event from the Cloudformation service.
// NOOP for now
func List(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	// client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	// if err != nil {
	// 	return handler.ProgressEvent{}, err
	// }

	// projectID := *currentModel.ProjectId
	// containerRequest := &matlasClient.ContainersListOptions{
	// 	ProviderName: *currentModel.ProviderName,
	// 	ListOptions:  matlasClient.ListOptions{},
	// }
	// containerResponse, _, err := client.Containers.List(context.Background(), projectID, containerRequest)
	// var models []Model
	// for _, container := range containerResponse {
	// 	var model Model
	// 	model.RegionName = &container.RegionName
	// 	model.Provisioned = container.Provisioned
	// 	model.VpcId = &container.VPCID
	// 	model.AtlasCidrBlock = &container.AtlasCIDRBlock

	// 	models = append(models, model)
	// }
	var models []Model
	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "List Complete",
		ResourceModel:   models,
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
	parameterStorePrefix := "mongodbstpatlasv1networkcontainer"
	return fmt.Sprintf("%s-%s", parameterStorePrefix, resourcePrimaryIdentifier)
}
