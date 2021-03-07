package resource

import (
	"context"
	"fmt"

	"github.com/422158/mongodbstpatlas-cloudformation-resources/cfn-resources/V1/project-ip-access-list/cmd/util"
	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
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

	return handler.ProgressEvent{
		OperationStatus: handler.Success,
		Message:         "Create Complete",
		ResourceModel:   currentModel,
	}, nil
}

// Read handles the Read event from the Cloudformation service.
func Read(req handler.Request, prevModel *Model, currentModel *Model) (handler.ProgressEvent, error) {
	client, err := util.CreateMongoDBClient(*currentModel.ApiKeys.PublicKey, *currentModel.ApiKeys.PrivateKey)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	projectID := *currentModel.ProjectId

	entries := []string{}
	for _, al := range currentModel.AccessList {
		entry := getEntry(al)
		entries = append(entries, entry)
	}

	accessList, err := getProjectIPAccessList(projectID, entries, client)
	if err != nil {
		return handler.ProgressEvent{}, err
	}

	currentModel.AccessList = flattenAccessList(accessList)

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

	err = deleteEntries(currentModel, client)
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
		return handler.ProgressEvent{
			OperationStatus: handler.Failed,
			Message:         "Delete Failed",
		}, err
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
