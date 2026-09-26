package hit_test

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omniaura/go-kit/errs"
	"github.com/omniaura/go-kit/net/hit"
)

// A type that declares its own wire encoding: every Body/Do of it is XML.
type quoteReq struct {
	XMLName xml.Name `xml:"QuoteRequest"`
	Symbol  string   `xml:"symbol"`
}

func (*quoteReq) DataType() hit.DataType { return hit.XML }

type quoteRsp struct {
	XMLName xml.Name `xml:"Quote"`
	Price   string   `xml:"price"`
}

func (*quoteRsp) DataType() hit.DataType { return hit.XML }

type soapFault struct {
	XMLName xml.Name `xml:"Fault"`
	Code    string   `xml:"faultcode"`
}

func TestHasDataTypeEncodesAndDecodesXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/xml" {
			t.Errorf("Content-Type = %q", ct)
		}
		if accept := r.Header.Get("Accept"); accept != "application/xml" {
			t.Errorf("Accept = %q, want the response type's encoding", accept)
		}
		var in quoteReq
		if err := xml.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = io.WriteString(w, `<Quote><price>42.00</price></Quote>`)
	}))
	t.Cleanup(srv.Close)

	c := hit.NewClient[hit.AnyError](srv.URL)
	req := quoteReq{Symbol: "DITO"}
	var rsp quoteRsp
	if err := c.POST("/quote").Body(&req).Do(context.Background(), &rsp); err != nil || rsp.Price != "42.00" {
		t.Fatalf("rsp = %+v, err = %v", rsp, err)
	}
}

func TestBodyAsFormAndResponseAsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", ct)
		}
		_ = r.ParseForm()
		_, _ = io.WriteString(w, "token-for-"+r.PostForm.Get("client_id"))
	}))
	t.Cleanup(srv.Close)

	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {"abc"}}
	var token string
	err := hit.POST[hit.AnyError](srv.URL).BodyAs(hit.Form, &form).ResponseAs(hit.Text).Do(context.Background(), &token)
	if err != nil || token != "token-for-abc" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
}

func TestClientDataTypeCoversErrorBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `<Fault><faultcode>Server.Busy</faultcode></Fault>`)
	}))
	t.Cleanup(srv.Close)

	// An all-XML API: the SDK sets the encoding once; soapFault declares none.
	c := hit.NewClient[soapFault](srv.URL).DataType(hit.XML).Service("soap")
	var fault soapFault
	var rsp quoteRsp
	err := c.GET("/quote").ErrorInto(&fault).Do(context.Background(), &rsp)
	if !hit.ErrUpstreamUnavailable.Is(err) {
		t.Fatalf("want upstream unavailable, got %v", err)
	}
	if fault.Code != "Server.Busy" {
		t.Fatalf("XML error body not decoded: %+v", fault)
	}
	if body, ok := errs.AsError(context.Background(), err).UpstreamAs[soapFault](); !ok || body.Code != "Server.Busy" {
		t.Fatalf("UpstreamAs = %+v, %v", body, ok)
	}
}

func TestDataTypeRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		dt   hit.DataType
		in   any
		out  any
		want string
	}{
		{hit.Form, &map[string]string{"a": "1"}, new(map[string]string), "a=1"},
		{hit.Text, new("hello"), new(string), "hello"},
		{hit.Bytes, &[]byte{1, 2}, new([]byte), "\x01\x02"},
	} {
		b, err := tc.dt.Marshal(tc.in)
		if err != nil || string(b) != tc.want {
			t.Fatalf("%s marshal = %q, %v", tc.dt.ContentType(), b, err)
		}
		if err := tc.dt.Unmarshal(b, tc.out); err != nil {
			t.Fatalf("%s unmarshal: %v", tc.dt.ContentType(), err)
		}
	}
	if _, err := hit.Form.Marshal(42); err == nil {
		t.Fatal("Form must reject values that are not maps, url.Values or structs")
	}
}

// OAuth-style: the request is form-encoded, the answer and the errors are JSON.
type tokenReq struct {
	GrantType string   `form:"grant_type"`
	ClientID  string   `form:"client_id"`
	Scope     []string `form:"scope,omitempty"`
	Secret    string   `form:"-"`
}

func (*tokenReq) DataType() hit.DataType { return hit.Form }

type tokenRsp struct {
	AccessToken string `json:"access_token"`
}

type oauthError struct {
	Error string `json:"error"`
}

func formInJSONOut(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want form", ct)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("Accept = %q, want json: the form request default must not leak into the response", accept)
		}
		_ = r.ParseForm()
		if r.PostForm.Has("Secret") || r.PostForm.Has("secret") {
			t.Errorf(`form:"-" field was sent: %v`, r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("client_id") == "bad" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"tok-`+r.PostForm.Get("client_id")+`-`+strings.Join(r.PostForm["scope"], "+")+`"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFormRequestJSONResponse_TypeDeclaresForm(t *testing.T) {
	c := hit.NewClient[oauthError](formInJSONOut(t).URL)

	req := tokenReq{GrantType: "client_credentials", ClientID: "abc", Scope: []string{"read", "write"}, Secret: "s3cret"}
	var rsp tokenRsp
	if err := c.POST("/token").Body(&req).Do(context.Background(), &rsp); err != nil || rsp.AccessToken != "tok-abc-read+write" {
		t.Fatalf("rsp = %+v, err = %v", rsp, err)
	}

	req.ClientID = "bad"
	var errRsp oauthError
	err := c.POST("/token").Body(&req).ErrorInto(&errRsp).Do(context.Background(), &rsp)
	if err == nil || errRsp.Error != "invalid_client" {
		t.Fatalf("JSON error body after a form request: errRsp = %+v, err = %v", errRsp, err)
	}
}

func TestFormRequestJSONResponse_ClientRequestDataType(t *testing.T) {
	// The request type declares nothing; the SDK says "we post forms".
	type plainTokenReq struct {
		ClientID string `json:"client_id"` // json tag name is the fallback
	}
	c := hit.NewClient[oauthError](formInJSONOut(t).URL).RequestDataType(hit.Form)
	var rsp tokenRsp
	if err := c.POST("/token").Body(&plainTokenReq{ClientID: "xyz"}).Do(context.Background(), &rsp); err != nil || rsp.AccessToken != "tok-xyz-" {
		t.Fatalf("rsp = %+v, err = %v", rsp, err)
	}
}

func TestDataTypeSetsBothDirections(t *testing.T) {
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		_, _ = io.WriteString(w, "a=1")
	}))
	t.Cleanup(srv.Close)
	var rsp map[string]string
	if err := hit.NewClient[hit.AnyError](srv.URL).DataType(hit.Form).GET("/").Do(context.Background(), &rsp); err != nil || rsp["a"] != "1" {
		t.Fatalf("rsp = %v, err = %v", rsp, err)
	}
	if accept != "application/x-www-form-urlencoded" {
		t.Fatalf("DataType should set the response default too, Accept = %q", accept)
	}
}

type level string

func (l level) MarshalText() ([]byte, error) { return []byte("L-" + string(l)), nil }

func TestFormStructEncoding(t *testing.T) {
	type Paging struct {
		Page int `form:"page"`
	}
	type req struct {
		Paging
		Name     string   `form:"name"`
		Nickname *string  `form:"nick"`
		Empty    string   `form:"empty,omitempty"`
		Tags     []string `form:"tag"`
		Ratio    float64  `form:"ratio"`
		OK       bool     `form:"ok"`
		Level    level    `form:"level"`
		hidden   string
	}
	b, err := hit.Form.Marshal(&req{Paging: Paging{Page: 2}, Name: "a b", Tags: []string{"x", "y"}, Ratio: 0.5, OK: true, Level: "hi", hidden: "no"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := url.ParseQuery(string(b))
	want := url.Values{"page": {"2"}, "name": {"a b"}, "tag": {"x", "y"}, "ratio": {"0.5"}, "ok": {"true"}, "level": {"L-hi"}}
	if got.Encode() != want.Encode() {
		t.Fatalf("form = %q, want %q", got.Encode(), want.Encode())
	}
	if _, err := hit.Form.Marshal(&struct {
		M map[string]int `form:"m"`
	}{M: map[string]int{"a": 1}}); err == nil {
		t.Fatal("unsupported field types must error, not be dropped silently")
	}
}
