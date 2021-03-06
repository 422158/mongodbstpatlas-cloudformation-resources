package testutil

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/davecgh/go-spew/spew"
)

type TestCase struct {
	Name        string
	Steps       []TestStep
	TestHandler TestHandler
}

type TestStep struct {
	Config string
	Check  TestCheckFunc
}

type TestCheckFunc func(model interface{}) error

// TestT is the interface used to handle the test lifecycle of a test.
//
// Users should just use a *testing.T object, which implements this.
type TestT interface {
	Error(args ...interface{})
	Fatal(args ...interface{})
	Skip(args ...interface{})
	Name() string
	Parallel()
}

func ComposeTestCheckFunc(fs ...TestCheckFunc) TestCheckFunc {
	return func(model interface{}) error {
		for i, f := range fs {
			if err := f(model); err != nil {
				return fmt.Errorf("Check %d/%d error: %s", i+1, len(fs), err)
			}
		}
		return nil
	}
}

func TestCheckResourceAttr(key string, value interface{}) TestCheckFunc {
	return func(model interface{}) error {
		data, err := json.Marshal(model)
		if err != nil {
			return err
		}

		d := map[string]interface{}{}
		err = json.Unmarshal(data, &d)
		if err != nil {
			return err
		}

		v, ok := d[key]
		if !ok {
			return fmt.Errorf("Key %s not found", key)
		}
		if v != value {
			return fmt.Errorf("Given value %v is different from configuration value %v", value, v)
		}

		return nil
	}
}

func Test(t TestT, ts TestCase) {
	log.Printf("[INFO] Running case %s\n", ts.Name)

	var model interface{}
	var data []byte
	for i, test := range ts.Steps {
		log.Printf("[DEBUG] Test: Executing step %d", i)
		if model == nil {
			data = []byte(test.Config)
			req := handler.NewRequest("id", map[string]interface{}{}, handler.RequestContext{}, &session.Session{}, nil, data)
			h := ts.TestHandler.Create(req)

			var err error
			h, err = checkStatus(h, ts.TestHandler.Create)
			if err != nil {
				t.Error(fmt.Sprintf("Error performing CREATE Operation: %s", err))
				return
			}

			if h.OperationStatus != handler.Failed {
				return
			}

			log.Println("[DEBUG] Running READ Operation")
			dataRead, err := json.Marshal(h.ResourceModel)
			if err != nil {
				t.Error(fmt.Sprintf("Error unmarshaling READ data %s", err))
				return
			}

			req = handler.NewRequest("id", h.CallbackContext, handler.RequestContext{}, &session.Session{}, nil, dataRead)
			hRead := ts.TestHandler.Read(req)
			if hRead.OperationStatus != handler.Success {
				t.Error(fmt.Sprintf("Error Performing READ Request %s: %s", err, h.Message))
				return
			}

			if equal := reflect.DeepEqual(h.ResourceModel, hRead.ResourceModel); !equal {
				want := spew.Sdump(h.ResourceModel)
				got := spew.Sdump(hRead.ResourceModel)
				t.Error(fmt.Sprintf("Mismatch between CREATE and READ want %s, got %s", want, got))
			}

			model = h.ResourceModel
		}

		if _, err := runTestStepChecks(model, test); err != nil {
			t.Error(fmt.Sprintf("Error in check: %s", err))
		}

	}
	if model != nil {
		log.Printf("[WARN] Test: Executing Delete step")
		data, err := json.Marshal(model)
		if err != nil {
			t.Error(fmt.Sprintf("[ERROR] Test: Error marshaling resource %s", err))
			return
		}
		req := handler.NewRequest("id", map[string]interface{}{}, handler.RequestContext{}, &session.Session{}, nil, data)
		h := ts.TestHandler.Delete(req)
		h, err = checkStatus(h, ts.TestHandler.Delete)
		if err != nil {
			t.Error("Error performing DELETE Operation %s: %s", err, h.Message)
		}
	}
}

func checkStatus(h handler.ProgressEvent, op Operation) (handler.ProgressEvent, error) {
	var err error

	switch h.OperationStatus {
	case handler.Success:
		return h, nil
	case handler.InProgress:
		h, err = waitForSuccess(h, op)
		if err != nil {
			return h, err
		}
	case handler.Failed:
		return h, fmt.Errorf("Failed with %s", h.Message)

	}

	return h, nil
}

func runTestStepChecks(model interface{}, step TestStep) (interface{}, error) {
	if step.Check != nil {
		if err := step.Check(model); err != nil {
			return model, err
		}
	}
	return model, nil
}

func waitForSuccess(h handler.ProgressEvent, op Operation) (handler.ProgressEvent, error) {
	for h.OperationStatus != handler.Success {
		d, err := time.ParseDuration(fmt.Sprintf("%ds", h.CallbackDelaySeconds))
		if err != nil {
			return h, fmt.Errorf("Failed to get duration: %s", err)
		}

		time.Sleep(d)

		data, err := json.Marshal(h.ResourceModel)
		if err != nil {
			return h, err
		}
		ctx := h.CallbackContext
		req := handler.NewRequest("id", ctx, handler.RequestContext{}, &session.Session{}, nil, data)
		h = op(req)
		if h.OperationStatus == handler.Failed {
			return h, fmt.Errorf("Failed performing operation: %s", h.Message)
		}

		log.Printf("[INFO] Operation has status %s", h.OperationStatus)
	}
	return h, nil
}
