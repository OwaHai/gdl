package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Dot-Rar/gdl/rest/routes"
	"github.com/pasztorpisti/qs"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const BASE_URL = "https://discordapp.com/api/v6"

type Endpoint struct {
	RequestType       RequestType
	ContentType       ContentType
	Endpoint          string
	AdditionalHeaders map[string]string
}

func (e *Endpoint) Request(token string, ratelimiter *routes.Ratelimiter, body interface{}, response interface{}) (error, *http.Response) {
	// Ratelimiter
	ch := make(chan struct{})
	go ratelimiter.Queue(ch)
	<-ch

	url := BASE_URL + e.Endpoint
	// Create req
	var req *http.Request
	var err error
	if body == nil || e.ContentType == Nil {
		req, err = http.NewRequest(string(e.RequestType), url, nil)
	} else {
		contentType := string(e.ContentType)

		// Encode body
		var encoded []byte
		if e.ContentType == ApplicationJson {
			raw, err := json.Marshal(body)
			if err != nil {
				return err, nil
			}
			encoded = raw
		} else if e.ContentType == ApplicationFormUrlEncoded {
			str, err := qs.Marshal(body)
			if err != nil {
				return err, nil
			}
			encoded = []byte(str)
		} else if e.ContentType == MultipartFormData {
			data, ok := body.(MultipartData); if !ok {
				return errors.New("Content-Type MultipartFormData specified but EncodeMultipartFormData was missing"), nil
			}

			var boundary string
			encoded, boundary, err = data.EncodeMultipartFormData(); if err != nil {
				return err, nil
			}

			contentType = fmt.Sprintf("%s; boundary=%s", MultipartFormData, boundary)
		}

		buff := bytes.NewBuffer(encoded)
		req, err = http.NewRequest(string(e.RequestType), url, buff)
		req.Header.Set("Content-Type", contentType)
	}

	if err != nil {
		return err, nil
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bot %s", token))

	for key, value := range e.AdditionalHeaders {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	client.Timeout = 3 * time.Second

	res, err := client.Do(req)
	if err != nil {
		return err, nil
	}
	defer res.Body.Close()

	applyNewRatelimits(res.Header, ratelimiter)

	content, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err, nil
	}

	if res.StatusCode < 200 || res.StatusCode > 226 {
		return errors.New(fmt.Sprintf("http status %d at %s: %s", res.StatusCode, e.Endpoint, string(content))), nil
	}

	return json.Unmarshal(content, response), res
}

func applyNewRatelimits(header http.Header, ratelimiter *routes.Ratelimiter) {
	ratelimiter.Lock()

	if limit, err := strconv.Atoi(header.Get("X-Ratelimit-Limit")); err == nil {
		ratelimiter.Limit = limit
	}

	if remaining, err := strconv.Atoi(header.Get("X-Ratelimit-Remaining")); err == nil {
		ratelimiter.Remaining = remaining
	}

	if resetAfter, err := strconv.Atoi(header.Get("X-Ratelimit-Reset-After")); err == nil {
		ratelimiter.Reset = time.Now().Unix() + int64(resetAfter)
	}

	bucket := header.Get("X-Ratelimit-Bucket")
	if bucket != "" {
		ratelimiter.Bucket = bucket
	}

	ratelimiter.Unlock()
}