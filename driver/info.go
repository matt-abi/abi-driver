package driver

import (
	"io/ioutil"
	"strings"

	"github.com/ability-sh/abi-lib/json"
	"gopkg.in/yaml.v2"
)

func GetConfig(p string) (interface{}, error) {

	b, err := ioutil.ReadFile(p)

	if err != nil {
		return nil, err
	}

	var info interface{} = nil

	if strings.HasSuffix(p, ".yaml") {

		err = yaml.Unmarshal(b, &info)

		if err != nil {
			return nil, err
		}

	} else {

		err = json.Unmarshal(b, &info)

		if err != nil {
			return nil, err
		}

	}

	return info, nil

}

func GetAppInfo() (interface{}, error) {

	info, err := GetConfig("./app.json")

	if err != nil {
		return GetConfig("./app.yaml")
	}

	return info, nil
}
