package parameterstore_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/torreirow/parsh/parameterstore"
)

var EddardStark = &ssm.Parameter{
	Name:  aws.String("/House/Stark/EddardStark"),
	Type:  aws.String("String"),
	Value: aws.String("Lord"),
}

var CatelynStark = &ssm.Parameter{
	Name:  aws.String("/House/Stark/CatelynStark"),
	Type:  aws.String("String"),
	Value: aws.String("Lady"),
}

var RobStark = &ssm.Parameter{
	Name:  aws.String("/House/Stark/RobStark"),
	Type:  aws.String("String"),
	Value: aws.String("Noble"),
}

var JonSnow = &ssm.Parameter{
	Name:  aws.String("/House/Stark/JonSnow"),
	Type:  aws.String("String"),
	Value: aws.String("Bastard"),
}

var DaenerysTargaryen = &ssm.Parameter{
	Name:  aws.String("/House/Targaryen/DaenerysTargaryen"),
	Type:  aws.String("String"),
	Value: aws.String("Noble"),
}

var HouseStark = []*ssm.Parameter{
	EddardStark,
	CatelynStark,
	RobStark,
}

var HouseTargaryen = []*ssm.Parameter{
	DaenerysTargaryen,
}

const NextToken = "A1B2C3D4"

type mockedSSM struct {
	ssmiface.SSMAPI
	GetParametersByPathResp ssm.GetParametersByPathOutput
	GetParametersByPathNext ssm.GetParametersByPathOutput
	GetParameterHistoryResp ssm.GetParameterHistoryOutput
	GetParametersResp       ssm.GetParametersOutput
	GetParameterResp        []ssm.GetParameterOutput
	DeleteParametersResp    ssm.DeleteParametersOutput
	PutParameterResp        ssm.PutParameterOutput
}

func (m mockedSSM) GetParametersByPath(in *ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error) {
	if aws.StringValue(in.NextToken) != "" {
		return &m.GetParametersByPathNext, nil
	}
	return &m.GetParametersByPathResp, nil
}

func (m mockedSSM) DeleteParameters(in *ssm.DeleteParametersInput) (*ssm.DeleteParametersOutput, error) {
	return &m.DeleteParametersResp, nil
}

func (m mockedSSM) GetParameter(in *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
	parameterName := aws.StringValue(in.Name)
	for _, param := range m.GetParameterResp {
		if aws.StringValue(param.Parameter.Name) == parameterName {
			return &param, nil
		}
	}
	return nil, errors.New("Parameter not found")
}

func (m mockedSSM) GetParameterHistory(in *ssm.GetParameterHistoryInput) (*ssm.GetParameterHistoryOutput, error) {
	return &m.GetParameterHistoryResp, nil
}

func (m mockedSSM) GetParameters(in *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
	for _, n := range in.Names {
		input := &ssm.GetParameterInput{
			Name:           n,
			WithDecryption: aws.Bool(true),
		}
		parameter, err := m.GetParameter(input)
		if err != nil {
			m.GetParametersResp.InvalidParameters = append(m.GetParametersResp.InvalidParameters, n)
		} else {
			m.GetParametersResp.Parameters = append(m.GetParametersResp.Parameters, parameter.Parameter)
		}
	}
	return &m.GetParametersResp, nil
}

func (m mockedSSM) PutParameter(in *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
	return &m.PutParameterResp, nil
}

func TestPut(t *testing.T) {
	var expectedVersion int64 = 1
	var p parameterstore.ParameterStore
	err := p.NewParameterStore(false)
	if err != nil {
		t.Fatal(err)
	}
	p.Cwd = parameterstore.Delimiter
	p.Clients[p.Region] = mockedSSM{
		PutParameterResp: ssm.PutParameterOutput{
			Version: aws.Int64(expectedVersion),
		},
	}
	putParameterInput := ssm.PutParameterInput{
		Name:        aws.String("/House/Stark/EddardStark"),
		Value:       aws.String("Lord"),
		Description: aws.String("Lord of Winterfell in Season 1"),
		Type:        aws.String("String"),
	}
	resp, err := p.Put(&putParameterInput, p.Region)
	if err != nil {
		t.Fatal("Error putting parameter", err)
	} else {
		if aws.Int64Value(resp.Version) != expectedVersion {
			msg := fmt.Errorf("expected %d, got %d", expectedVersion, aws.Int64Value(resp.Version))
			t.Fatal(msg)
		}
	}
}

func TestMoveParameter(t *testing.T) {
	srcParam := parameterstore.ParameterPath{
		Name:   "/House/Stark/SansaStark",
		Region: "region",
	}
	dstParam := parameterstore.ParameterPath{
		Name:   "/House/Lannister/SansaStark",
		Region: "region",
	}
	var p parameterstore.ParameterStore
	p.Region = "region"
	err := p.NewParameterStore(false)
	if err != nil {
		t.Fatal(err)
	}
	p.Cwd = parameterstore.Delimiter
	p.Clients[p.Region] = mockedSSM{
		GetParameterResp: []ssm.GetParameterOutput{
			{
				Parameter: &ssm.Parameter{
					Name:  aws.String(srcParam.Name),
					Type:  aws.String("String"),
					Value: aws.String("Noble"),
				},
			},
			{
				Parameter: &ssm.Parameter{
					Name:  aws.String(dstParam.Name),
					Type:  aws.String("String"),
					Value: aws.String("Noble"),
				},
			},
		},
		GetParameterHistoryResp: ssm.GetParameterHistoryOutput{
			Parameters: []*ssm.ParameterHistory{
				{
					Name:        aws.String(srcParam.Name),
					Value:       aws.String("Noble"),
					Type:        aws.String("String"),
					Description: aws.String("Eldest daughter of House Stark, bethrothed to Tyrion Lannister"),
					Version:     aws.Int64(2),
				},
				{
					Name:        aws.String(srcParam.Name),
					Value:       aws.String("Noble"),
					Type:        aws.String("String"),
					Description: aws.String("Eldest daughter of House Stark"),
					Version:     aws.Int64(1),
				},
			},
		},
	}
	err = p.Move(srcParam, dstParam)
	if err != nil {
		t.Fatal("Error moving parameter", err)
	}
	p.Clients[p.Region] = mockedSSM{
		GetParameterResp: []ssm.GetParameterOutput{
			{
				Parameter: &ssm.Parameter{
					Name:  aws.String(dstParam.Name),
					Type:  aws.String("String"),
					Value: aws.String("Noble"),
				},
			},
		},
	}
	resp, err := p.Get([]string{srcParam.Name}, p.Region, false)
	if err != nil {
		msg := fmt.Errorf("Error getting %s: %s", srcParam.Name, err)
		t.Fatal(msg)
	}
	if len(resp) > 0 {
		if err != nil {
			msg := fmt.Errorf("Expected parameter %s to be removed but found %v", srcParam.Name, resp)
			t.Fatal(msg)
		}
	}
	_, err = p.Get([]string{dstParam.Name}, p.Region, false)
	if err != nil {
		msg := fmt.Errorf("Expected to find %s but didn't", dstParam.Name)
		t.Fatal(msg)
	}
}

func TestCopyPath(t *testing.T) {
	srcPath := parameterstore.ParameterPath{
		Name:   "/House/Stark",
		Region: "region",
	}
	dstPath := parameterstore.ParameterPath{
		Name:   "/House/Targaryen",
		Region: "region",
	}

	var p parameterstore.ParameterStore
	p.Region = "region"
	err := p.NewParameterStore(false)
	if err != nil {
		t.Fatal(err)
	}
	p.Cwd = parameterstore.Delimiter
	bothHouses := append(HouseStark, HouseTargaryen...)
	p.Clients[p.Region] = mockedSSM{
		GetParameterResp: []ssm.GetParameterOutput{
			{Parameter: EddardStark},
			{Parameter: CatelynStark},
			{Parameter: RobStark},
			{Parameter: JonSnow},
			{Parameter: DaenerysTargaryen},
		},
		GetParametersByPathResp: ssm.GetParametersByPathOutput{
			Parameters: bothHouses,
			NextToken:  aws.String(NextToken),
		},
		GetParametersByPathNext: ssm.GetParametersByPathOutput{
			Parameters: []*ssm.Parameter{JonSnow},
			NextToken:  aws.String(""),
		},
		GetParameterHistoryResp: ssm.GetParameterHistoryOutput{
			Parameters: []*ssm.ParameterHistory{
				{
					Name:    aws.String("/House/Stark/EddardStark"),
					Version: aws.Int64(2),
				},
			},
			NextToken: aws.String(""),
		},
	}
	err = p.Copy(srcPath, dstPath, true)
	if err != nil {
		t.Fatal("Error copying parameter path: ", err)
	}
	expectedName := parameterstore.ParameterPath{
		Name:   "/House/Targaryen/Stark/EddardStark",
		Region: "region",
	}
	resp, err := p.GetHistory(expectedName)
	if err != nil {
		t.Fatal("Error getting history: ", err)
	}
	if len(resp) != 1 {
		msg := fmt.Errorf("Expected history of length 1, got %s", resp)
		t.Fatal(msg)
	}
}

func TestCopyParameter(t *testing.T) {
	srcParam := parameterstore.ParameterPath{
		Name:   "/House/Stark/JonSnow",
		Region: "region",
	}
	dstParam := parameterstore.ParameterPath{
		Name:   "/House/Targaryen/JonSnow",
		Region: "region",
	}
	var p parameterstore.ParameterStore
	p.Region = "region"
	err := p.NewParameterStore(false)
	if err != nil {
		t.Fatal(err)
	}
	p.Cwd = parameterstore.Delimiter
	p.Clients[p.Region] = mockedSSM{
		GetParameterResp: []ssm.GetParameterOutput{
			{
				Parameter: &ssm.Parameter{
					Name:  aws.String("/House/Stark/JonSnow"),
					Type:  aws.String("String"),
					Value: aws.String("King"),
				},
			},
			{
				Parameter: &ssm.Parameter{
					Name:  aws.String("/House/Targaryen/JonSnow"),
					Type:  aws.String("String"),
					Value: aws.String("King"),
				},
			},
		},
		GetParameterHistoryResp: ssm.GetParameterHistoryOutput{
			Parameters: []*ssm.ParameterHistory{
				{
					Name:        aws.String("/House/Stark/JonSnow"),
					Value:       aws.String("King"),
					Type:        aws.String("String"),
					Description: aws.String("King of the north"),
					Version:     aws.Int64(2),
				},
				{
					Name:        aws.String("/House/Stark/JonSnow"),
					Value:       aws.String("Bastard"),
					Type:        aws.String("String"),
					Description: aws.String("Bastard of Winterfell"),
					Version:     aws.Int64(1),
				},
			},
		},
	}
	err = p.Copy(srcParam, dstParam, false)
	if err != nil {
		t.Fatal("Error copying parameter", err)
	}
	resp, err := p.Get([]string{dstParam.Name}, p.Region, false)
	if err != nil {
		t.Fatal("Error getting parameter", err)
	}
	expectedName := parameterstore.ParameterPath{
		Name:   "/House/Targaryen/JonSnow",
		Region: "region",
	}
	if aws.StringValue(resp[0].Name) != expectedName.Name {
		msg := fmt.Errorf("expected %s, got %s", expectedName.Name, aws.StringValue(resp[0].Name))
		t.Fatal(msg)
	}
}

func TestCwd(t *testing.T) {
	cases := []struct {
		GetParametersByPathResp ssm.GetParametersByPathOutput
		Path                    string
		Expected                string
	}{
		{
			Path:     "/",
			Expected: "/",
		},
		{
			Path: "/House/Stark/..///Deceased",
			GetParametersByPathResp: ssm.GetParametersByPathOutput{
				Parameters: []*ssm.Parameter{
					{
						Name:  aws.String("/House/Stark/EddardStark"),
						Type:  aws.String("String"),
						Value: aws.String("Lord"),
					},
				},
				NextToken: aws.String(""),
			},
			Expected: "/House/Deceased",
		},
	}

	var p parameterstore.ParameterStore
	for _, c := range cases {
		err := p.NewParameterStore(false)
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		p.Region = "region"
		p.Cwd = parameterstore.Delimiter
		p.Clients[p.Region] = mockedSSM{
			GetParametersByPathResp: c.GetParametersByPathResp,
		}
		err = p.SetCwd(parameterstore.ParameterPath{Name: c.Path, Region: "region"})
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		if p.Cwd != c.Expected {
			msg := fmt.Errorf("expected %v, got %v", c.Expected, p.Cwd)
			t.Fatal(msg)
		}
	}

	err := p.NewParameterStore(false)
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	p.Cwd = parameterstore.Delimiter
	testDir := parameterstore.ParameterPath{
		Name:   "/nodir",
		Region: "region",
	}
	err = p.SetCwd(testDir)
	if err == nil {
		msg := fmt.Errorf("Expected error for dir %s, got cwd %s ", testDir, p.Cwd)
		t.Fatal(msg)
	}
}

func TestDelete(t *testing.T) {
	testParams := []parameterstore.ParameterPath{
		{
			Name:   "/House/Stark/EddardStark",
			Region: "region",
		},
		{
			Name:   "/House/Stark/CatelynStark",
			Region: "region",
		},
		{
			Name:   "/House/Stark/TyrionLannister",
			Region: "region",
		},
	}
	deleteParametersOutput := ssm.DeleteParametersOutput{
		DeletedParameters: []*string{
			aws.String("/House/Stark/EddardStark"),
			aws.String("/House/Stark/CatelynStark"),
		},
		InvalidParameters: []*string{
			aws.String("/House/Stark/TyrionLannister"),
		},
	}

	var p parameterstore.ParameterStore
	p.Region = "region"
	err := p.NewParameterStore(false)
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	p.Clients[p.Region] = mockedSSM{
		DeleteParametersResp: deleteParametersOutput,
	}
	err = p.Remove(testParams, false)
	if err == nil {
		msg := fmt.Errorf("Expected error for param %s, got %v ", testParams[2], err)
		t.Fatal(msg)
	}
}

func TestGetHistory(t *testing.T) {
	testParam := parameterstore.ParameterPath{
		Name:   "/House/Stark/EddardStark",
		Region: "region",
	}
	getHistoryOutput := ssm.GetParameterHistoryOutput{
		Parameters: []*ssm.ParameterHistory{
			{
				Name:    aws.String("/House/Stark/EddardStark"),
				Version: aws.Int64(2),
			},
			{
				Name:    aws.String("/House/Stark/EddardStark"),
				Version: aws.Int64(1),
			},
		},
		NextToken: aws.String(""),
	}
	var p parameterstore.ParameterStore
	p.Region = "region"
	err := p.NewParameterStore(false)
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	p.Clients[p.Region] = mockedSSM{
		GetParameterHistoryResp: getHistoryOutput,
	}
	resp, err := p.GetHistory(testParam)
	if err != nil {
		msg := fmt.Errorf("Unexpected error %s", err)
		t.Fatal(msg)
	}
	if len(resp) != 2 {
		msg := fmt.Errorf("Expected history of length 2, got %s", resp)
		t.Fatal(msg)
	}
}

func TestList(t *testing.T) {
	cases := []struct {
		Query                   parameterstore.ParameterPath
		GetParametersByPathResp ssm.GetParametersByPathOutput
		GetParametersResp       ssm.GetParametersOutput
		GetParametersByPathNext ssm.GetParametersByPathOutput
		Expected                []string
		Recurse                 bool
	}{
		{
			Query: parameterstore.ParameterPath{
				Name:   "/House/Stark/EddardStark",
				Region: "region",
			},
			Recurse: false,
			GetParametersByPathResp: ssm.GetParametersByPathOutput{
				Parameters: []*ssm.Parameter{},
				NextToken:  aws.String(""),
			},
			Expected: []string{
				"/House/Stark/EddardStark",
			},
			GetParametersResp: ssm.GetParametersOutput{
				Parameters: []*ssm.Parameter{
					{
						Name:  aws.String("/House/Stark/EddardStark"),
						Type:  aws.String("String"),
						Value: aws.String("Lord"),
					},
				},
			},
		}, {
			Query: parameterstore.ParameterPath{
				Name:   "/",
				Region: "region",
			},
			Recurse: false,
			Expected: []string{
				"root",
			},
			GetParametersResp: ssm.GetParametersOutput{
				Parameters: []*ssm.Parameter{
					{
						Name:  aws.String("root"),
						Type:  aws.String("String"),
						Value: aws.String("A root parameter"),
					},
				},
			},
		},
		{
			Query: parameterstore.ParameterPath{
				Name:   "/House/Stark",
				Region: "region",
			},
			Recurse: false,
			GetParametersByPathResp: ssm.GetParametersByPathOutput{
				Parameters: HouseStark,
				NextToken:  aws.String(""),
			},
			Expected: []string{
				"EddardStark",
				"CatelynStark",
				"RobStark",
			},
		},
		{
			Query: parameterstore.ParameterPath{
				Name:   "/House/",
				Region: "region",
			},
			Recurse: true,
			GetParametersByPathResp: ssm.GetParametersByPathOutput{
				Parameters: HouseStark,
				NextToken:  aws.String(NextToken),
			},
			GetParametersByPathNext: ssm.GetParametersByPathOutput{
				Parameters: []*ssm.Parameter{JonSnow, DaenerysTargaryen},
				NextToken:  aws.String(""),
			},
			Expected: []string{
				"/House/Stark/EddardStark",
				"/House/Stark/CatelynStark",
				"/House/Stark/RobStark",
				"/House/Stark/JonSnow",
				"/House/Targaryen/DaenerysTargaryen",
			},
		},
	}

	for _, c := range cases {
		var p parameterstore.ParameterStore
		p.Region = "region"
		err := p.NewParameterStore(false)
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		p.Clients[p.Region] = mockedSSM{
			GetParametersByPathResp: c.GetParametersByPathResp,
			GetParametersByPathNext: c.GetParametersByPathNext,
			GetParametersResp:       c.GetParametersResp,
		}
		p.Cwd = parameterstore.Delimiter

		ch := make(chan parameterstore.ListResult)
		quit := make(chan bool)
		go func() {
			p.List(c.Query, c.Recurse, ch, quit)
		}()

		result := <-ch
		if result.Error != nil {
			quit <- true
			t.Fatal("unexpected error", result.Error)
		}
		if !equal(result.Result, c.Expected) {
			msg := fmt.Errorf("expected %v, got %v", c.Expected, result.Result)
			t.Fatal(msg)
		}
	}
}

// mockedSSMCapture extends the mock to capture the WithDecryption flag passed to GetParameters
type mockedSSMCapture struct {
	ssmiface.SSMAPI
	GetParameterResp       []ssm.GetParameterOutput
	capturedWithDecryption *bool
}

func (m *mockedSSMCapture) GetParameter(in *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
	for _, param := range m.GetParameterResp {
		if aws.StringValue(param.Parameter.Name) == aws.StringValue(in.Name) {
			return &param, nil
		}
	}
	return nil, errors.New("Parameter not found")
}

func (m *mockedSSMCapture) GetParameters(in *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
	m.capturedWithDecryption = in.WithDecryption
	resp := ssm.GetParametersOutput{}
	for _, n := range in.Names {
		parameter, err := m.GetParameter(&ssm.GetParameterInput{Name: n})
		if err != nil {
			resp.InvalidParameters = append(resp.InvalidParameters, n)
		} else {
			resp.Parameters = append(resp.Parameters, parameter.Parameter)
		}
	}
	return &resp, nil
}

// TestGetDecryptTrueReturnsPlaintext verifies that Get() returns plaintext when Decrypt=true and the mock provides it
func TestGetDecryptTrueReturnsPlaintext(t *testing.T) {
	testParam := parameterstore.ParameterPath{Name: "/House/Stark/AryaStark", Region: "region"}
	var p parameterstore.ParameterStore
	p.Region = "region"
	p.Decrypt = true
	if err := p.NewParameterStore(false); err != nil {
		t.Fatal(err)
	}
	p.Cwd = parameterstore.Delimiter
	p.Clients[p.Region] = &mockedSSMCapture{
		GetParameterResp: []ssm.GetParameterOutput{
			{Parameter: &ssm.Parameter{
				Name:  aws.String(testParam.Name),
				Type:  aws.String("SecureString"),
				Value: aws.String("Faceless"),
			}},
		},
	}
	resp, err := p.Get([]string{testParam.Name}, p.Region, p.Decrypt)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if len(resp) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(resp))
	}
	if aws.StringValue(resp[0].Value) != "Faceless" {
		t.Fatalf("Expected plaintext value 'Faceless', got %q", aws.StringValue(resp[0].Value))
	}
}

// TestGetDecryptTrueEncryptedPassthrough verifies that Get() passes through <encrypted> when the API returns it
// (detection and warning is the responsibility of commands/get.go)
func TestGetDecryptTrueEncryptedPassthrough(t *testing.T) {
	testParam := parameterstore.ParameterPath{Name: "/House/Stark/AryaStark", Region: "region"}
	var p parameterstore.ParameterStore
	p.Region = "region"
	p.Decrypt = true
	if err := p.NewParameterStore(false); err != nil {
		t.Fatal(err)
	}
	p.Cwd = parameterstore.Delimiter
	p.Clients[p.Region] = &mockedSSMCapture{
		GetParameterResp: []ssm.GetParameterOutput{
			{Parameter: &ssm.Parameter{
				Name:  aws.String(testParam.Name),
				Type:  aws.String("SecureString"),
				Value: aws.String("<encrypted>"),
			}},
		},
	}
	resp, err := p.Get([]string{testParam.Name}, p.Region, p.Decrypt)
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if len(resp) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(resp))
	}
	if aws.StringValue(resp[0].Value) != "<encrypted>" {
		t.Fatalf("Expected <encrypted> pass-through, got %q", aws.StringValue(resp[0].Value))
	}
}

// TestGetWithDecryptionFlagPassthrough verifies that WithDecryption is correctly passed to the SSM API
func TestGetWithDecryptionFlagPassthrough(t *testing.T) {
	testParam := parameterstore.ParameterPath{Name: "/House/Stark/BranStark", Region: "region"}
	paramOutput := ssm.GetParameterOutput{Parameter: &ssm.Parameter{
		Name:  aws.String(testParam.Name),
		Type:  aws.String("SecureString"),
		Value: aws.String("TheThreeEyedRaven"),
	}}

	for _, decrypt := range []bool{true, false} {
		var p parameterstore.ParameterStore
		p.Region = "region"
		p.Decrypt = decrypt
		if err := p.NewParameterStore(false); err != nil {
			t.Fatal(err)
		}
		p.Cwd = parameterstore.Delimiter
		mock := &mockedSSMCapture{GetParameterResp: []ssm.GetParameterOutput{paramOutput}}
		p.Clients[p.Region] = mock

		if _, err := p.Get([]string{testParam.Name}, p.Region, decrypt); err != nil {
			t.Fatalf("Unexpected error (decrypt=%v): %v", decrypt, err)
		}
		if mock.capturedWithDecryption == nil {
			t.Fatalf("WithDecryption was not captured (decrypt=%v)", decrypt)
		}
		if aws.BoolValue(mock.capturedWithDecryption) != decrypt {
			t.Fatalf("Expected WithDecryption=%v, got %v", decrypt, aws.BoolValue(mock.capturedWithDecryption))
		}
	}
}

func equal(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for i := 0; i < len(first); i++ {
		if first[i] != second[i] {
			return false
		}
	}
	return true
}
