package driver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/matt-abi/abi-lib/errors"
	"github.com/matt-abi/abi-lib/eval"
	"github.com/matt-abi/abi-micro/micro"
)

type RouteSchemeItem struct {
	Alias     string                            `json:"alias"`
	Title     string                            `json:"title"`
	Scheme    micro.IScheme                     `json:"scheme"`
	GetScheme func(micro.Context) micro.IScheme `json:"-"`
}

type routeInterceptorItem struct {
	Match       func(name string) (string, bool)
	Interceptor func(ctx micro.Context, name string, data interface{}) error
}

type RouteScheme struct {
	Items []*RouteSchemeItem `json:"items"`
}

type routerItem struct {
	Match func(name string) (string, bool)
	Exec  func(ctx micro.Context, name string, data interface{}) (interface{}, error)
}

type Router struct {
	respHandlers map[string]micro.RespFunc
	verifiers    map[string]micro.ReqVerifyFunc
	items        []*routerItem
	interceptors []*routeInterceptorItem
	scheme       *RouteScheme
}

func NewRouter() *Router {
	return &Router{scheme: &RouteScheme{}}
}

func (R *Router) Add(match func(name string) (string, bool), executor micro.Executor) *Router {
	R.items = append(R.items, &routerItem{Match: match, Exec: func(ctx micro.Context, name string, data interface{}) (interface{}, error) {
		return executor.Exec(ctx, name, data)
	}})
	R.scheme.Items = append(R.scheme.Items, &RouteSchemeItem{Alias: "",
		GetScheme: func(ctx micro.Context) micro.IScheme { return executor.Scheme(ctx) }})
	return R
}

func (R *Router) Rewrite(pattern *regexp.Regexp, to string, executor micro.Executor) *Router {
	R.items = append(R.items, &routerItem{Match: func(name string) (string, bool) {
		vs := pattern.FindStringSubmatch(name)
		n := len(vs)
		if n > 0 {
			dst := eval.ParseEval(to, func(key string) string {
				i, _ := strconv.Atoi(key)
				if i < n {
					return vs[i]
				}
				return key
			})
			return dst, true
		}
		return "", false
	}, Exec: func(ctx micro.Context, name string, data interface{}) (interface{}, error) {
		return executor.Exec(ctx, name, data)
	}})
	R.scheme.Items = append(R.scheme.Items, &RouteSchemeItem{Alias: "", Title: fmt.Sprintf("rewrite %s %s", pattern.String(), to),
		GetScheme: func(ctx micro.Context) micro.IScheme { return executor.Scheme(ctx) }})
	return R
}

func (R *Router) Use(pattern *regexp.Regexp, executor micro.Executor) *Router {
	R.items = append(R.items, &routerItem{Match: func(name string) (string, bool) {
		if pattern.MatchString(name) {
			return name, true
		}
		return "", false
	}, Exec: func(ctx micro.Context, name string, data interface{}) (interface{}, error) {
		return executor.Exec(ctx, name, data)
	}})
	R.scheme.Items = append(R.scheme.Items, &RouteSchemeItem{Alias: "", Title: fmt.Sprintf("regex %s", pattern.String()),
		GetScheme: func(ctx micro.Context) micro.IScheme { return executor.Scheme(ctx) }})
	return R
}

func (R *Router) Alias(alias string, executor micro.Executor) *Router {
	n := len(alias)
	R.items = append(R.items, &routerItem{Match: func(name string) (string, bool) {
		if strings.HasPrefix(name, alias) {
			return name[n:], true
		}
		return "", false
	}, Exec: func(ctx micro.Context, name string, data interface{}) (interface{}, error) {
		return executor.Exec(ctx, name, data)
	}})
	R.scheme.Items = append(R.scheme.Items, &RouteSchemeItem{Alias: alias,
		GetScheme: func(ctx micro.Context) micro.IScheme { return executor.Scheme(ctx) }})
	return R
}

func (R *Router) Service(alias string, serviceName string) *Router {
	n := len(alias)
	R.items = append(R.items, &routerItem{Match: func(name string) (string, bool) {
		if strings.HasPrefix(name, alias) {
			return name[n:], true
		}
		return "", false
	}, Exec: func(ctx micro.Context, name string, data interface{}) (interface{}, error) {
		e, err := ctx.Runtime().GetExecutor(serviceName)
		if err != nil {
			return nil, err
		}
		return e.Exec(ctx, name, data)
	}})
	R.scheme.Items = append(R.scheme.Items, &RouteSchemeItem{Alias: alias, GetScheme: func(ctx micro.Context) micro.IScheme {
		e, err := ctx.Runtime().GetExecutor(serviceName)
		if err != nil {
			return nil
		}
		return e.Scheme(ctx)
	}})
	return R
}

func (R *Router) Exec(ctx micro.Context, name string, data interface{}) (interface{}, error) {

	for _, item := range R.interceptors {
		dst, ok := item.Match(name)
		if ok {
			err := item.Interceptor(ctx, dst, data)
			if err != nil {
				return nil, err
			}
		}
	}

	for _, item := range R.items {
		dst, ok := item.Match(name)
		if ok {
			return item.Exec(ctx, dst, data)
		}
	}
	return nil, errors.Errorf(404, "not found")
}

func (R *Router) Scheme(ctx micro.Context) micro.IScheme {
	for _, item := range R.scheme.Items {
		if item.GetScheme != nil {
			item.Scheme = item.GetScheme(ctx)
		}
	}
	return R.scheme
}

func (R *Router) MatchRespHandler(ctx micro.Context, name string) micro.RespFunc {
	return R.respHandlers[name]
}

func (R *Router) MatchReqVerify(ctx micro.Context, name string) micro.ReqVerifyFunc {
	return R.verifiers[name]
}

func (R *Router) Interceptor(pattern *regexp.Regexp, interceptor func(ctx micro.Context, name string, data interface{}) error) *Router {
	R.interceptors = append(R.interceptors, &routeInterceptorItem{Match: func(name string) (string, bool) {
		if pattern.MatchString(name) {
			return name, true
		}
		return "", false
	}, Interceptor: interceptor})
	return R
}

func (R *Router) RespHander(name string, handler micro.RespFunc) *Router {
	if R.respHandlers == nil {
		R.respHandlers = make(map[string]micro.RespFunc, 10)
	}
	R.respHandlers[name] = handler
	return R
}

func (R *Router) ReqVerify(name string, verify micro.ReqVerifyFunc) *Router {
	if R.verifiers == nil {
		R.verifiers = make(map[string]micro.ReqVerifyFunc, 10)
	}
	R.verifiers[name] = verify
	return R
}
