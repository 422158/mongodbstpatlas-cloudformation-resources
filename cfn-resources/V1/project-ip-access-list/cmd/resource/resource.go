package resource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/project-ip-access-list/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/rs/xid"
	"go.mongodb.org/atlas/mongodbatlas"
)

// Create handles the Create event from the Cloudformation service.
func Create(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}
	fmt.Printf("%#+v\n", currentModel)

	err = createEntries(currentModel, client)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	guid := xid.New()

	x := guid.String()
	currentModel.Id = &x
	// putting api keys and project name into parameter store (needed for read operation)
	_, err = putParameterIntoParameterStore(currentModel.Id, &ParameterToBePersistedSpec{ApiKeys: currentModel.ApiKeys, ProjectId: currentModel.ProjectId}, req.Session)
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
// read handler is NO-OP for now as we do not need it, in case of need we can easily implement it here
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	// client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	// if err != nil {
	// 	return handler.ProgressEvent{}, err
	// }

	// projectID := *currentModel.ProjectId

	// entries := []string{}
	// for _, al := range currentModel.AccessList {
	// 	entry := getEntry(al)
	// 	entries = append(entries, entry)
	// }

	// accessList, err := getProjectIPAccessList(projectID, entries, client)
	// if err != nil {
	// 	return handler.ProgressEvent{}, err
	// }

	// currentModel.AccessList = flattenAccessList(accessList)

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

	err = deleteEntries(prevModel, client)
	if err != nil {
		return handler.ProgressEvent{
			OperationStatus: handler.Failed,
			Message:         "Update Failed",
		}, err
	}

	err = createEntries(currentModel, client)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	currentModel.Id = prevModel.Id

	// putting api keys and project name into parameter store (needed for read operation)
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

	err = deleteEntries(currentModel, client)
	if err != nil {
		// even when error occurs when deleting, we still want to delete parameter from parameter store
		_, errParams := deleteParameterFromParameterStore(currentModel.Id, req.Session)
		if err != nil {
			return handler.ProgressEvent{
				OperationStatus:  handler.Failed,
				Message:          "Delete Failed",
				HandlerErrorCode: "GeneralServiceException",
			}, fmt.Errorf("Error deleting project ip access list: %s.\nError deleting api keys from parameter store: %s", err, errParams)
		}
		return handler.ProgressEvent{
			OperationStatus: handler.Failed,
			Message:         "Delete Failed",
		}, err
	}

	_, err = deleteParameterFromParameterStore(currentModel.Id, req.Session)
	if err != nil {
		return handler.ProgressEvent{
			OperationStatus: handler.Failed,
			Message:         "Delete Failed",
		}, fmt.Errorf("error deleting api keys from parameter store: %s", err)
	}

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Delete Complete",
		ResourceModel:   currentModel,
	}, nil
}

// List handles the List event from the Cloudformation service.
// NO-OP
func List(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "List Complete",
		ResourceModel:   currentModel,
	}, nil
}

func getProjectIPAccessList(projectID string, entries []string, conn *mongodbatlas.Client) ([]*mongodbatlas.ProjectIPAccessList, error) {

	var accessList []*mongodbatlas.ProjectIPAccessList
	for _, entry := range entries {
		res, _, err := conn.ProjectIPAccessList.Get(context.Background(), projectID, entry)
		if err != nil {
			return nil, fmt.Errorf("error getting project IP accessList information: %s", err)
		}
		accessList = append(accessList, res)
	}
	return accessList, nil
}

func getProjectIPAccessListRequest(model *Model) []*mongodbatlas.ProjectIPAccessList {
	var accessList []*mongodbatlas.ProjectIPAccessList
	for _, modelEntry := range model.AccessList {
		wl := &mongodbatlas.ProjectIPAccessList{}
		if modelEntry.Comment != nil {
			wl.Comment = *modelEntry.Comment
		}
		if modelEntry.CidrBlock != nil {
			wl.CIDRBlock = *modelEntry.CidrBlock
		}
		if modelEntry.IpAddress != nil {
			wl.IPAddress = *modelEntry.IpAddress
		}
		if modelEntry.AwsSecurityGroup != nil {
			wl.AwsSecurityGroup = *modelEntry.AwsSecurityGroup
		}

		fmt.Printf("%+#v\n", wl)

		accessList = append(accessList, wl)
	}
	return accessList
}

func getEntry(al AccessListEntryDefinition) string {
	if al.IpAddress != nil {
		return *al.IpAddress
	}
	if al.CidrBlock != nil {
		return *al.CidrBlock
	}
	if al.AwsSecurityGroup != nil {
		return *al.AwsSecurityGroup
	}
	return ""
}

func flattenAccessList(accessList []*mongodbatlas.ProjectIPAccessList) []AccessListEntryDefinition {
	var results []AccessListEntryDefinition
	for _, al := range accessList {
		r := AccessListEntryDefinition{
			IpAddress:        &al.IPAddress,
			CidrBlock:        &al.CIDRBlock,
			AwsSecurityGroup: &al.AwsSecurityGroup,
			Comment:          &al.Comment,
			ProjectId:        &al.GroupID,
		}
		results = append(results, r)
	}
	return results
}

func createEntries(model *Model, client *mongodbatlas.Client) error {
	request := getProjectIPAccessListRequest(model)
	projectID := *model.ProjectId

	_, _, err := client.ProjectIPAccessList.Create(context.Background(), projectID, request)
	return err
}

func deleteEntries(model *Model, client *mongodbatlas.Client) error {
	projectID := *model.ProjectId
	var err error

	for _, al := range model.AccessList {
		entry := getEntry(al)
		_, errDelete := client.ProjectIPAccessList.Delete(context.Background(), projectID, entry)
		if errDelete != nil {
			err = fmt.Errorf("error deleting(%s) %w ", entry, errDelete)
		}
	}

	return err
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
	networkPeeringParameterStorePrefix := "mongodbstpatlasv1projectipaccesslist"
	return fmt.Sprintf("%s-%s", networkPeeringParameterStorePrefix, resourcePrimaryIdentifier)
}
