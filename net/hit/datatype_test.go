package hit_test

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if _, err := hit.Form.Marshal(struct{}{}); err == nil {
		t.Fatal("Form must reject types it cannot encode")
	}
}
