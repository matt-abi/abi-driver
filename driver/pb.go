package driver

import (
	"github.com/ability-sh/abi-lib/dynamic"
	"github.com/ability-sh/abi-lib/errors"
)

type PBResult interface {
	GetErrno() int32
	GetErrmsg() string
}

func GetResult(rs interface{}, err error) (interface{}, error) {
	if err == nil {
		e, ok := rs.(PBResult)
		if ok && e.GetErrno() != 200 {
			return nil, errors.Errorf(e.GetErrno(), "%s", e.GetErrmsg())
		}
	}
	return rs, err
}

func GetData(rs interface{}, err error) (interface{}, error) {
	rs, err = GetResult(rs, err)
	if err != nil {
		return nil, err
	}
	return dynamic.Get(rs, "data"), err
}

func MergeData(rs interface{}, err error) (interface{}, error) {
	rs, err = GetResult(rs, err)
	if err != nil {
		return nil, err
	}
	data := map[string]interface{}{}
	dynamic.Each(rs, func(key interface{}, value interface{}) bool {
		skey := dynamic.StringValue(key, "")
		if skey == "errno" || skey == "errmsg" {
			return true
		}
		data[skey] = value
		return true
	})
	return data, err
}
