package unit

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ability-sh/abi-ac-driver/driver"
	"github.com/ability-sh/abi-lib/dynamic"
	"github.com/ability-sh/abi-lib/errors"
	"github.com/ability-sh/abi-lib/json"
	"github.com/ability-sh/abi-micro/micro"
	"github.com/ability-sh/abi-micro/runtime"
	unit "unit.nginx.org/go"
)

func Run(executor micro.Executor) error {

	AC_APPID := os.Getenv("AC_APPID")
	AC_VER := os.Getenv("AC_VER")
	AC_ABILITY := os.Getenv("AC_ABILITY")

	AC_ENV := os.Getenv("AC_ENV")
	AC_ADDR := os.Getenv("AC_ADDR")
	AC_CONFIG := os.Getenv("AC_CONFIG")
	AC_LOG_FILE := os.Getenv("AC_LOG_FILE")
	AC_HTTP_BODY_SIZE, _ := strconv.ParseInt(os.Getenv("AC_HTTP_BODY_SIZE"), 10, 64)

	if AC_HTTP_BODY_SIZE == 0 {
		AC_HTTP_BODY_SIZE = 1024 * 1024 * 500
	}

	if AC_LOG_FILE != "" {
		fd, err := os.OpenFile(AC_LOG_FILE, os.O_APPEND, os.ModeAppend)
		if err != nil {
			fd, err = os.Create(AC_LOG_FILE)
		}
		if err != nil {
			log.Panicln(err)
		}
		os.Stdout = fd
		os.Stderr = fd
	}

	var config interface{} = nil
	var err error = nil

	if AC_ENV == "unit" {

		err = json.Unmarshal([]byte(AC_CONFIG), &config)

		if err != nil {
			return err
		}

	} else {

		config, err = driver.GetConfig("./config.yaml")

		if err != nil {
			return err
		}
	}

	p := runtime.NewPayload()

	err = p.SetConfig(config)

	if err != nil {
		return err
	}

	defer p.Exit()

	info, _ := driver.GetAppInfo()

	getConfigValue := func(key string) interface{} {
		v := dynamic.Get(config, key)
		if v == nil {
			v = dynamic.Get(info, key)
		}
		return v
	}

	alias := dynamic.StringValue(getConfigValue("alias"), "/")

	if !strings.HasSuffix(alias, "/") {
		alias = alias + "/"
	}

	alias_n := len(alias)

	s_state := alias + "__stat"
	s_scheme := alias + "__scheme"

	http.HandleFunc(alias, func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path == s_state {
			setDataResponse(w, map[string]interface{}{"appid": AC_APPID, "ver": AC_VER, "ability": AC_ABILITY, "env": AC_ENV})
			return
		}

		if r.URL.Path == s_scheme {

			trace := r.Header.Get("Trace")

			if trace == "" {
				r.Header.Get("trace")
			}

			if trace == "" {
				trace = micro.NewTrace()
				w.Header().Add("Trace", trace)
			}

			ctx, err := p.NewContext("__scheme", trace)

			if err != nil {
				setErrorResponse(w, err)
				return
			}

			defer ctx.Recycle()

			setDataResponse(w, executor.Scheme(ctx))

			return
		}

		if strings.HasSuffix(r.URL.Path, ".json") {

			var name = r.URL.Path[alias_n:]

			trace := r.Header.Get("Trace")

			if trace == "" {
				r.Header.Get("trace")
			}

			if trace == "" {
				trace = micro.NewTrace()
				w.Header().Add("Trace", trace)
			}

			dynamic.Each(getConfigValue("cors"), func(key interface{}, value interface{}) bool {
				k := dynamic.StringValue(key, "")
				v := dynamic.StringValue(value, "")
				if k == "Access-Control-Allow-Origin" {
					if v == "*" {
						referer := r.Header.Get("Referer")
						if referer == "" {
							referer = r.Header.Get("referer")
						}
						if referer != "" {
							u, _ := url.Parse(referer)
							if u != nil {
								v = u.Scheme + "://" + u.Host
							}
						}
					} else {
						referer := r.Header.Get("Referer")
						if referer == "" {
							referer = r.Header.Get("referer")
						}
						if referer != "" {
							u, _ := url.Parse(referer)
							if u != nil {
								s := u.Scheme + "://" + u.Host
								for _, ss := range strings.Split(v, ",") {
									if s == ss {
										v = s
										break
									}
								}
							}
						}
					}
				}
				w.Header().Add(k, v)
				return true
			})

			sessionKey := dynamic.StringValue(getConfigValue("sessionKey"), "abi-ac")

			if r.Method == "OPTIONS" || r.Method == "HEAD" {
				w.Write([]byte{})
				return
			}

			ctx, err := p.NewContext(name, trace)

			if err != nil {
				setErrorResponse(w, err)
				return
			}

			defer ctx.Recycle()

			clientIp := getClientIp(r)
			sessionId := getSessionId(r, w, sessionKey)

			ctx.SetValue("clientIp", clientIp)
			ctx.SetValue("sessionId", sessionId)

			ctx.AddTag("clientIp", clientIp)
			ctx.AddTag("sessionId", sessionId)

			var inputData interface{} = nil
			ctype := r.Header.Get("Content-Type")

			if ctype == "" {
				ctype = r.Header.Get("content-type")
			}

			if strings.Contains(ctype, "multipart/form-data") {
				inputData = map[string]interface{}{}
				r.ParseMultipartForm(AC_HTTP_BODY_SIZE)
				if r.MultipartForm != nil {
					for key, values := range r.MultipartForm.Value {
						dynamic.Set(inputData, key, values[0])
					}
					for key, values := range r.MultipartForm.File {
						dynamic.Set(inputData, key, values[0])
					}
				}
			} else if strings.Contains(ctype, "json") {

				b, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()

				if err == nil {
					json.Unmarshal(b, &inputData)
				}

			} else {

				inputData = map[string]interface{}{}

				r.ParseForm()

				for key, values := range r.Form {
					dynamic.Set(inputData, key, values[0])
				}

			}

			ctx.Step("input")("%+v", inputData)

			rs, err := executor.Exec(ctx, name, inputData)

			if err != nil {
				setErrorResponse(w, err)
				return
			}

			setDataResponse(w, rs)

			return
		}

		w.WriteHeader(404)
		w.Write([]byte("Not Found"))
	})

	if AC_ADDR == "" {
		AC_ADDR = ":8084"
	}

	if AC_ENV == "unit" {
		return unit.ListenAndServe(AC_ADDR, nil)
	} else {
		log.Println("HTTPD", AC_ADDR)
		return http.ListenAndServe(AC_ADDR, nil)
	}

}

func setErrorResponse(w http.ResponseWriter, err error) {

	{
		he, ok := err.(*micro.HttpContent)
		if ok {
			for k, v := range he.Headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(he.Code)
			w.Write(he.Body)
			return
		}
	}

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	e, ok := err.(*errors.Error)
	if ok {
		b, _ := json.Marshal(e)
		w.Write(b)
	} else {
		b, _ := json.Marshal(map[string]interface{}{"errno": 500, "errmsg": err.Error()})
		w.Write(b)
	}
}

func setDataResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	b, _ := json.Marshal(map[string]interface{}{"errno": 200, "data": data})
	w.Write(b)
}

var clientKeys = []string{"X-Forwarded-For", "x-forwarded-for"}
var reg_clientIp, _ = regexp.Compile(`[0-9\.\:]+`)

func getClientIp(r *http.Request) string {

	for _, key := range clientKeys {
		v := r.Header.Get(key)
		if v != "" {
			return strings.Split(v, ",")[0]
		}
	}

	return reg_clientIp.FindString(strings.Split(r.RemoteAddr, ":")[0])
}

func getSessionId(r *http.Request, w http.ResponseWriter, sessionKey string) string {

	c, _ := r.Cookie(sessionKey)

	if c == nil {
		sessionId := micro.NewTrace()
		cookie := http.Cookie{
			Name:     sessionKey,
			Value:    sessionId,
			HttpOnly: true,
			Path:     "/",
		}
		http.SetCookie(w, &cookie)
		return sessionId
	} else {
		return c.Value
	}

}
