package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const BASE_URL = `https://exp.host`
const BASE_API_URL = `https://exp.host/--/api/v2`
const PUSH_NOTIFICATION_CHUNK_LIMIT = 100
const POST = "POST"
const GET = "GET"
const sdkVersion = "2.3.1"

type ExpoClientOptions struct {
	httpAgent HttpAgent
}

type HttpAgent map[string]string

type ExpoPushToken string
type ExpoClient struct {
	PushNotificationChunkSizeLimit int
	httpAgent                      HttpAgent
}
type MessageData struct {
	Type string `json:"type"`
	Data string `json:"data"`
}
type ExpoPushMessage struct {
	To         ExpoPushToken `json:"to"`
	Data       interface{}   `json:"data"`
	Title      string        `json:"title"`
	Body       string        `json:"body"`
	Sound      string        `json:"sound"` // 'default' | null
	Ttl        int64         `json:"ttl"`
	Expiration int64         `json:"expiration"`
	Priority   string        `json:"priority"` //'default' | 'normal' | 'high',
	Badge      int64         `json:"badge"`
}
type ExpoPushReceipt struct {
	Status  string //'ok' | 'error',
	Details struct {
		Error string // 'DeviceNotRegistered' | 'MessageTooBig' | 'MessageRateExceeded',
	}
	// Internal field used only by developers working on Expo
	//__debug?: any,
}
type RequestOptions struct {
	HttpMethod     string                 //'get' | 'post',
	Body           interface{}            // any,
	ShouldCompress func(body string) bool // (body: string) => boolean,
}

type ApiResult struct {
	Errors []ApiResultError
	Data   interface{} // any,
}

type ApiResultError struct {
	Message string
	Code    string
	Details interface{} // any,
	Stack   string
}

func (this *ExpoClient) isExpoPushToken(token string) bool {
	return strings.HasPrefix(token, "ExponentPushToken[") || strings.HasPrefix(token, "ExpoPushToken[") && strings.HasSuffix(token, "]")
}
func (this *ExpoClient) isExponentPushToken(token string) bool {
	return this.isExpoPushToken(token)
}
func (this *ExpoClient) sendPushNotificationAsync(message ExpoPushMessage) ExpoPushReceipt {
	var receipts = this.sendPushNotificationsAsync([]ExpoPushMessage{message})
	//invariant(receipts.length === 1, `Expected exactly one push receipt`);
	return receipts[0]
}
func (this *ExpoClient) sendPushNotificationsAsync(messages []ExpoPushMessage) []ExpoPushReceipt {
	var option = RequestOptions{
		HttpMethod: POST,
		Body:       messages,
		ShouldCompress: func(body string) bool {
			return len(body) > 1024
		},
	}
	var data = this._requestAsync(fmt.Sprintf(`%s/push/send`, BASE_API_URL), option)

	/*if (!Array.isArray(data) || data.length !== messages.length) {
	    let apiError: Object = new Error(
	      `Expected Exponent to respond with ${messages.length} ` +
	        `${messages.length === 1 ? 'receipt' : 'receipts'} but got ` +
	        `${data.length}`
	    );
	    apiError.data = data;
	    throw apiError;
	  }
	*/
	return data
}
func (this *ExpoClient) chunkPushNotifications(messages []ExpoPushMessage) [][]ExpoPushMessage {
	var chunks = [][]ExpoPushMessage{}
	var chunk = []ExpoPushMessage{}
	for _, message := range messages {
		chunk = append(chunk, message)
		if len(chunk) >= PUSH_NOTIFICATION_CHUNK_LIMIT {
			chunks = append(chunks, chunk)
			chunk = []ExpoPushMessage{}
		}
	}
	if len(chunk) > 0 {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func (this *ExpoClient) _requestAsync(url string, options RequestOptions) (result []ExpoPushReceipt) {
	/*var sdkVersion = "2.3.1"
	  let fetchOptions = {
	    method: options.httpMethod,
	    body: JSON.stringify(options.body),
	    headers: new Headers({
	      Accept: 'application/json',
	      'Accept-Encoding': 'gzip, deflate',
	      'User-Agent': fmt.Sprintf(`expo-server-sdk-node/%s`, sdkVersion)
	    }),
	    agent: this._httpAgent,
	  };
	  if (options.body != nil) {
	    let json = JSON.stringify(options.body);
	    invariant(json != null, `JSON request body must not be null`);
	    if (options.shouldCompress(json)) {
	      fetchOptions.body = await _gzipAsync(Buffer.from(json));
	      fetchOptions.headers.set('Content-Encoding', 'gzip');
	    } else {
	      fetchOptions.body = json;
	    }

	    fetchOptions.headers.set('Content-Type', 'application/json');
	  }*/
	headers := make(map[string]string)
	headers["Accept"] = "application/json"
	headers["Accept-Encoding"] = "gzip, deflate"
	headers["User-Agent"] = fmt.Sprintf(`expo-server-sdk-go/%s`, sdkVersion)
	headers["Content-Type"] = "application/json"

	body, _ := json.Marshal(options.Body)
	var params string
	if options.ShouldCompress(string(body)) {
		params = string(_gzipAsync(body))
		headers["Content-Encoding"] = "gzip"
	} else {
		params = string(body)
	}
	if options.HttpMethod == POST {
		//do post with header
		data, err := HttpPOSTWithHeader(url, params, headers, []*http.Cookie{}, "")
		fmt.Println(string(data), err)
		if err != nil {
			log.Println(err)
			return
		}
		var apiResult ApiResult
		err = json.Unmarshal(data, &apiResult)
		if err != nil {
			log.Println(err)
			return
		}
		result, _ = this._getExpoPushReceiptFromResult(apiResult.Data)
		return result

	} else {
		//do get with header
	}

	/*let response = await fetch(url, fetchOptions);

	  if (response.status !== 200) {
	    let apiError = await this._parseErrorResponseAsync(response);
	    throw apiError;
	  }

	  // We expect the API response body to be JSON
	  let result: ApiResult;
	  try {
	    result = await response.json();
	  } catch (e) {
	    let apiError = await this._getTextResponseErrorAsync(response);
	    throw apiError;
	  }

	  if (result.errors) {
	    let apiError = this._getErrorFromResult(result);
	    throw apiError;
	  }

	  return result.data;*/
	return []ExpoPushReceipt{}
}
func (this *ExpoClient) _getTextResponseErrorAsync(response ApiResult) []ApiResultError {
	return response.Errors
}
func (this *ExpoClient) _getErrorFromResult(result ApiResult) error {
	// invariant(
	//   result.errors && result.errors.length > 0,
	//   `Expected at least one error from Exponent`
	// );
	// let [errorData, ...otherErrorData] = result.errors;
	// let error: Object = this._getErrorFromResultError(errorData);
	// if (otherErrorData.length) {
	//   error.others = otherErrorData.map(data => this._getErrorFromResultError(data));
	// }
	// return error;
	return nil
}
func (this *ExpoClient) _getErrorFromResultError(errorData ApiResultError) error {
	var e = errors.New(errorData.Message)
	// error.code = errorData.code;

	// if (errorData.details != null) {
	//   error.details = errorData.details;
	// }

	// if (errorData.stack != null) {
	//   error.serverStack = errorData.stack;
	// }

	// return error;
	return e
}
func (this *ExpoClient) _getExpoPushReceiptFromResult(data interface{}) (result []ExpoPushReceipt, err error) {
	var json_str []byte
	json_str, err = json.Marshal(data)
	if err != nil {
		return
	}
	err = json.Unmarshal(json_str, &result)
	return
}
func _gzipAsync(data []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Name = "_gzipAsync"
	zw.Comment = "exponent"
	zw.ModTime = time.Now()
	_, err := zw.Write(data)
	if err != nil {
		log.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func HttpGet(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return []byte{}, err
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

func HttpPostForm(url string, params url.Values) ([]byte, error) {
	res, err := http.PostForm(url, params)
	if err != nil {
		return []byte{}, err
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}
func HttpGetWithHeader(requestURL string, headers map[string]string, cookies []*http.Cookie, proxy string) ([]byte, error) {
	log.Println("HttpGetWithHeader", requestURL, headers, cookies, proxy)
	req, err := http.NewRequest("GET", requestURL, nil)
	if len(headers) > 0 {
		for k, v := range headers {
			req.Header.Add(k, v)
		}
	}
	if len(cookies) > 0 {
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
	}

	var client *http.Client
	client = &http.Client{}
	if len(proxy) > 0 {
		proxyUrl, err := url.Parse(fmt.Sprintf("http://%s", proxy))
		if err == nil {
			transport := &http.Transport{}
			transport.Proxy = http.ProxyURL(proxyUrl)                         // set proxy
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //set ssl
			client.Transport = transport
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

func HttpPOSTWithHeader(requestURL string, params string, headers map[string]string, cookies []*http.Cookie, proxy string) ([]byte, error) {
	log.Println("HttpPOSTWithHeader", requestURL, params, headers, cookies, proxy)
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(params))

	if len(headers) > 0 {
		for k, v := range headers {
			req.Header.Add(k, v)
		}
	}
	if len(cookies) > 0 {
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
	}
	var client *http.Client
	client = &http.Client{}
	log.Println("+++++++++++++++++++++++++++++++++++++++")
	if len(proxy) > 0 {
		if !strings.Contains(proxy, "http") {
			proxy = fmt.Sprintf("http://%s", proxy)
		}
		proxyUrl, err := url.Parse(proxy)
		if err == nil {
			transport := &http.Transport{}
			transport.Proxy = http.ProxyURL(proxyUrl)                         // set proxy
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //set ssl
			client.Transport = transport
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}
func main() {
	fmt.Println("main")
	var expo ExpoClient
	var message ExpoPushMessage
	var messageContent MessageData
	messageContent.Type = "product"
	messageContent.Data = "product content"
	message.To = `ExponentPushToken[qYfJV_MScsgVu4mfmZ7VdE]`
	message.Data = messageContent
	message.Sound = "default"
	message.Body = "test body"
	message.Title = "chào các bạn"
	message.Priority = "default"
	result := expo.sendPushNotificationAsync(message)
	fmt.Println("result", result)
}
