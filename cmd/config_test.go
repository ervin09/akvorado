package cmd_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v2"

	"akvorado/cmd"
	"akvorado/common/helpers"
)

type dummyConfiguration struct {
	Module1 dummyModule1Configuration
	Module2 dummyModule2Configuration
}
type dummyModule1Configuration struct {
	Listen  string
	Topic   string
	Workers int
}
type dummyModule2Configuration struct {
	Details     dummyModule2DetailsConfiguration
	Elements    []dummyModule2ElementsConfiguration
	MoreDetails `mapstructure:",squash" yaml:",inline"`
}
type MoreDetails struct {
	Stuff string
}
type dummyModule2ElementsConfiguration struct {
	Name  string
	Gauge int
}
type dummyModule2DetailsConfiguration struct {
	Workers       int
	IntervalValue time.Duration
}

func dummyDefaultConfiguration() dummyConfiguration {
	return dummyConfiguration{
		Module1: dummyModule1Configuration{
			Listen:  "127.0.0.1:8080",
			Topic:   "nothingness",
			Workers: 100,
		},
		Module2: dummyModule2Configuration{
			MoreDetails: MoreDetails{
				Stuff: "hello",
			},
			Details: dummyModule2DetailsConfiguration{
				Workers:       1,
				IntervalValue: time.Minute,
			},
		},
	}
}

func TestDump(t *testing.T) {
	// Configuration file
	config := `---
module1:
 topic: flows
module2:
 details:
  workers: 5
  interval-value: 20m
 stuff: bye
 elements:
  - name: first
    gauge: 67
  - name: second
`
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	ioutil.WriteFile(configFile, []byte(config), 0644)

	c := cmd.ConfigRelatedOptions{
		Path: configFile,
		Dump: true,
	}

	parsed := dummyDefaultConfiguration()
	out := bytes.NewBuffer([]byte{})
	if err := c.Parse(out, "dummy", &parsed); err != nil {
		t.Fatalf("Parse() error:\n%+v", err)
	}
	// Expected configuration
	expected := dummyConfiguration{
		Module1: dummyModule1Configuration{
			Listen:  "127.0.0.1:8080",
			Topic:   "flows",
			Workers: 100,
		},
		Module2: dummyModule2Configuration{
			MoreDetails: MoreDetails{
				Stuff: "bye",
			},
			Details: dummyModule2DetailsConfiguration{
				Workers:       5,
				IntervalValue: 20 * time.Minute,
			},
			Elements: []dummyModule2ElementsConfiguration{
				{"first", 67},
				{"second", 0},
			},
		},
	}
	if diff := helpers.Diff(parsed, expected); diff != "" {
		t.Errorf("Parse() (-got, +want):\n%s", diff)
	}

	var gotRaw map[string]map[string]interface{}
	if err := yaml.Unmarshal(out.Bytes(), &gotRaw); err != nil {
		t.Fatalf("Unmarshal() error:\n%+v", err)
	}
	expectedRaw := map[string]interface{}{
		"module1": map[string]interface{}{
			"listen":  "127.0.0.1:8080",
			"topic":   "flows",
			"workers": 100,
		},
		"module2": map[string]interface{}{
			"stuff": "bye",
			"details": map[string]interface{}{
				"workers":       5,
				"intervalvalue": "20m0s",
			},
			"elements": []interface{}{
				map[string]interface{}{
					"name":  "first",
					"gauge": 67,
				},
				map[string]interface{}{
					"name":  "second",
					"gauge": 0,
				},
			},
		},
	}
	if diff := helpers.Diff(gotRaw, expectedRaw); diff != "" {
		t.Errorf("Parse() (-got, +want):\n%s", diff)
	}
}

func TestEnvOverride(t *testing.T) {
	// Configuration file
	config := `---
module1:
 topic: flows
module2:
 details:
  workers: 5
  interval-value: 20m
`
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	ioutil.WriteFile(configFile, []byte(config), 0644)

	// Environment
	clean := func() {
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "AKVORADO_DUMMY_") {
				os.Unsetenv(strings.Split(env, "=")[0])
			}
		}
	}
	clean()
	defer clean()
	os.Setenv("AKVORADO_DUMMY_MODULE1_LISTEN", "127.0.0.1:9000")
	os.Setenv("AKVORADO_DUMMY_MODULE1_TOPIC", "something")
	os.Setenv("AKVORADO_DUMMY_MODULE2_DETAILS_INTERVALVALUE", "10m")
	os.Setenv("AKVORADO_DUMMY_MODULE2_STUFF", "bye")
	os.Setenv("AKVORADO_DUMMY_MODULE2_ELEMENTS_0_NAME", "something")
	os.Setenv("AKVORADO_DUMMY_MODULE2_ELEMENTS_0_GAUGE", "18")
	os.Setenv("AKVORADO_DUMMY_MODULE2_ELEMENTS_1_NAME", "something else")
	os.Setenv("AKVORADO_DUMMY_MODULE2_ELEMENTS_1_GAUGE", "7")

	c := cmd.ConfigRelatedOptions{
		Path: configFile,
		Dump: true,
	}

	parsed := dummyDefaultConfiguration()
	out := bytes.NewBuffer([]byte{})
	if err := c.Parse(out, "dummy", &parsed); err != nil {
		t.Fatalf("Parse() error:\n%+v", err)
	}
	// Expected configuration
	expected := dummyConfiguration{
		Module1: dummyModule1Configuration{
			Listen:  "127.0.0.1:9000",
			Topic:   "something",
			Workers: 100,
		},
		Module2: dummyModule2Configuration{
			MoreDetails: MoreDetails{
				Stuff: "bye",
			},
			Details: dummyModule2DetailsConfiguration{
				Workers:       5,
				IntervalValue: 10 * time.Minute,
			},
			Elements: []dummyModule2ElementsConfiguration{
				{"something", 18},
				{"something else", 7},
			},
		},
	}
	if diff := helpers.Diff(parsed, expected); diff != "" {
		t.Errorf("Parse() (-got, +want):\n%s", diff)
	}
}

func TestHTTPConfiguration(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
		fmt.Fprint(w, `---
module1:
 topic: flows
module2:
 details:
  workers: 5
  interval-value: 20m
 stuff: bye
 elements:
   - {"name": "first", "gauge": 67}
   - {"name": "second"}
`)
	}))
	defer ts.Close()

	c := cmd.ConfigRelatedOptions{
		Path: ts.URL,
		Dump: true,
	}

	parsed := dummyDefaultConfiguration()
	out := bytes.NewBuffer([]byte{})
	if err := c.Parse(out, "dummy", &parsed); err != nil {
		t.Fatalf("Parse() error:\n%+v", err)
	}
	// Expected configuration
	expected := dummyConfiguration{
		Module1: dummyModule1Configuration{
			Listen:  "127.0.0.1:8080",
			Topic:   "flows",
			Workers: 100,
		},
		Module2: dummyModule2Configuration{
			MoreDetails: MoreDetails{
				Stuff: "bye",
			},
			Details: dummyModule2DetailsConfiguration{
				Workers:       5,
				IntervalValue: 20 * time.Minute,
			},
			Elements: []dummyModule2ElementsConfiguration{
				{"first", 67},
				{"second", 0},
			},
		},
	}
	if diff := helpers.Diff(parsed, expected); diff != "" {
		t.Errorf("Parse() (-got, +want):\n%s", diff)
	}
}
