package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
)

func TestTrimSpaces(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single line is unchanged",
			input: "value",
			want:  "value",
		},
		{
			name:  "ishell multiline trailing spaces are removed",
			input: "line1 \nline2 \nline3",
			want:  "line1\nline2\nline3",
		},
		{
			name:  "inline multiline value keeps last character",
			input: "0 2 * * * /usr/bin/backup.sh\n30 3 * * 0 /usr/bin/clean.sh",
			want:  "0 2 * * * /usr/bin/backup.sh\n30 3 * * 0 /usr/bin/clean.sh",
		},
		{
			name:  "empty lines do not panic",
			input: "# backup\n\n0 2 * * * /usr/bin/backup.sh\n",
			want:  "# backup\n\n0 2 * * * /usr/bin/backup.sh\n",
		},
		{
			name:  "only one trailing space is removed",
			input: "a  \nb",
			want:  "a \nb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimSpaces(tt.input); got != tt.want {
				t.Errorf("trimSpaces(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateValueFromFile(t *testing.T) {
	content := "# backup\n\n0 2 * * * /usr/bin/backup.sh\n"
	fn := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(fn, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	putParamInput = ssm.PutParameterInput{}
	if err := validateValue("file://" + fn); err != nil {
		t.Fatalf("validateValue returned error: %v", err)
	}
	if got := aws.StringValue(putParamInput.Value); got != content {
		t.Errorf("value = %q, want %q", got, content)
	}
}

func TestValidateValueFromMissingFile(t *testing.T) {
	putParamInput = ssm.PutParameterInput{}
	fn := filepath.Join(t.TempDir(), "does-not-exist")
	if err := validateValue("file://" + fn); err == nil {
		t.Error("expected error for missing file, got nil")
	}
	if putParamInput.Value != nil {
		t.Errorf("value should not be set, got %q", aws.StringValue(putParamInput.Value))
	}
}
