package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/project/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	matlasClient "go.mongodb.org/atlas/mongodbatlas"
)

// Create handles the Create event from the Cloudformation service.
func Create(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	project, _, err := client.Projects.Create(context.Background(), &matlasClient.Project{
		Name:  *currentModel.Name,
		OrgID: *currentModel.OrgId,
	})
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error creating project: %s", err)
	}

	currentModel.Id = &project.ID
	currentModel.Created = &project.Created
	currentModel.ClusterCount = &project.ClusterCount

	// putting api keys and project name into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.Id, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys}, req.Session)
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

	params, err := getParameterFromParameterStore(currentModel.Id, req.Session)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	client, err := util.CreateMongoDBClient(*params.ApiKeys.PrivateKey, *params.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	id := *currentModel.Id
	log.Printf("Looking for project: %s", id)

	project, _, err := client.Projects.GetOneProject(context.Background(), id)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error reading project with id(%s): %s", id, err)
	}

	currentModel.Name = &project.Name
	currentModel.OrgId = &project.OrgID
	currentModel.Created = &project.Created
	currentModel.ClusterCount = &project.ClusterCount

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
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, err
	}

	id := *currentModel.Id
	log.Printf("Deleting project with id(%s)", id)

	_, err = client.Projects.Delete(context.Background(), id)
	if err != nil {
		// even when error occurs when deleting, we still want to delete parameter from parameter store
		_, errParams := deleteParameterFromParameterStore(currentModel.Id, req.Session)
		if err != nil {
			return handler.ProgressEvent{
				OperationStatus:  handler.Failed,
				Message:          "Delete Failed",
				HandlerErrorCode: "GeneralServiceException",
			}, fmt.Errorf("Error deleting project with id(%s): %s.\nError deleting api keys from parameter store: %s", id, err, errParams)
		}
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Error deleting project with id(%s): %s", id, err)
	}

	_, err = deleteParameterFromParameterStore(currentModel.Id, req.Session)
	if err != nil {
		return handler.ProgressEvent{
			OperationStatus:  handler.Failed,
			Message:          "Delete Failed",
			HandlerErrorCode: "GeneralServiceException",
		}, fmt.Errorf("Error deleting api keys from parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Delete Complete",
		ResourceModel:   currentModel,
	}, nil
}

// List handles the List event from the Cloudformation service.
func List(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	listOptions := &matlasClient.ListOptions{
		PageNum:      0,
		ItemsPerPage: 100,
	}
	projects, _, err := client.Projects.GetAllProjects(context.Background(), listOptions)
	if err != nil {
		return handler.ProgressEvent{}, fmt.Errorf("error retrieving projects: %s", err)
	}

	var models []Model
	for _, project := range projects.Results {
		var m Model
		m.Name = &project.Name
		m.OrgId = &project.OrgID
		m.Created = &project.Created
		m.ClusterCount = &project.ClusterCount
		m.Id = &project.ID

		models = append(models, m)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "List Complete",
		ResourceModel:   models,
	}, nil
}

type ParameterToBePersistedSpec struct {
	ApiKeys *ApiKeyDefinition
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
	networkPeeringParameterStorePrefix := "mongodbstpatlasv1project"
	return fmt.Sprintf("%s-%s", networkPeeringParameterStorePrefix, resourcePrimaryIdentifier)
}
