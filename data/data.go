package data

import (
	"io/ioutil"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v2"
)

type Attachment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type TaskYaml struct {
	Name        string
	Description string
	Flag        string
	Author      string
	Category    string
	Tags        []string
	Attachments []Attachment
	Host        *string
	Port        *int
	IsSurvey    bool `yaml:"is_survey"`
}

func Load(path string) (*TaskYaml, error) {
	// load task.yml
	taskb, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, xerrors.Errorf(": %w", err)
	}

	var tasky TaskYaml
	if err := yaml.Unmarshal(taskb, &tasky); err != nil {
		return nil, xerrors.Errorf(": %w", err)
	}

	hostStr := ""
	if tasky.Host != nil {
		hostStr = *tasky.Host
	}
	portStr := ""
	if tasky.Port != nil {
		portStr = strconv.FormatInt(int64(*tasky.Port), 10)
	}

	r := strings.NewReplacer("{host}", hostStr, "{port}", portStr)
	tasky.Description = r.Replace(tasky.Description)
	return &tasky, nil
}
