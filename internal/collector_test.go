package internal

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/robertkrimen/otto"
)

//go:embed samples/*
var samples embed.FS

func TestingDecryptPassword(password string) string {
	result := []byte{}
	for i := 1; i < len(password); i = i + 2 {
		result = append(result, byte(int(password[i])+(len(password)/2)))
	}
	return string(result)
}

func TestingHandleRequest(rw http.ResponseWriter, req *http.Request) {
	response := ""
	if req.URL.String() == "/login.cgi" && req.Method == http.MethodPost {
		if err := req.ParseForm(); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			response = err.Error()
		} else if TestingDecryptPassword(req.Form.Get("password")) != "OFcVQl1shaUM" {
			rw.WriteHeader(http.StatusUnauthorized)
		}
	} else if req.URL.String() == "/logout.html" {
	} else {
		data, err := samples.ReadFile("samples" + req.URL.String())
		if err != nil {
			rw.WriteHeader(http.StatusNotFound)
			response = err.Error()
		} else {
			response = string(data)
		}
	}
	_, _ = rw.Write([]byte(response))
}

func TestCollector_Login(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(TestingHandleRequest))
	// Close the server when test finishes
	defer server.Close()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "correct password",
			password: "OFcVQl1shaUM",
			wantErr:  false,
		},
		{
			name:     "wrong password",
			password: "Niovei4uR2ao",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Collector{
				address:  strings.Replace(server.URL, "http://", "", 1),
				password: tt.password,
			}
			if err := c.Login(); (err != nil) != tt.wantErr {
				t.Errorf("Collector.Login() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCollector_Logout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(TestingHandleRequest))
	// Close the server when test finishes
	defer server.Close()

	t.Run("logout", func(t *testing.T) {
		c := &Collector{
			address:  strings.Replace(server.URL, "http://", "", 1),
			password: "unused",
		}
		c.Logout()
	})
}

func TestCollector_EncryptPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		match    string
	}{
		{
			name:     "8 characters",
			password: "xue2That",
			match:    "^.p.m.].*.L.`.Y.l.$",
		},
		{
			name:     "12 characters",
			password: "Gae9ahVahzah",
			match:    "^.;.U.Y.-.U.\\\\.J.U.\\\\.n.U.\\\\.$",
		},
		{
			name:     "15 characters",
			password: "muthuj0Eicha3ah",
			match:    "^.\\^.f.e.Y.f.\\[.!.6.Z.T.Y.R.\\$.R.Y.$",
		},
	}
	c := &Collector{
		address:  "127.0.0.1",
		password: "secret",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.EncryptPassword(tt.password)
			match, err := regexp.MatchString(tt.match, got)
			if err != nil {
				t.Errorf("Collector.EncryptPassword() failed: %v", err)
			} else if !match {
				t.Errorf("Collector.EncryptPassword() = %v, should match %v", got, tt.match)
			}
		})
	}
}

func TestCollector_FetchJS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(TestingHandleRequest))
	// Close the server when test finishes
	defer server.Close()

	tests := []struct {
		filename string
		size     int
		wantErr  bool
	}{
		{
			filename: "link_data.js",
			size:     569,
			wantErr:  false,
		},
		{
			filename: "system_data.js",
			size:     634,
			wantErr:  false,
		},
		{
			filename: "VLAN_1Q_List_data.js",
			size:     166,
			wantErr:  false,
		},
		{
			filename: "world_peace.js",
			size:     0,
			wantErr:  true,
		},
	}
	c := &Collector{
		address:  strings.Replace(server.URL, "http://", "", 1),
		password: "secret",
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := c.FetchJS(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("Collector.FetchJS(%v) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.size {
				t.Errorf("Collector.FetchJS(%v) = %v bytes, want %v bytes", tt.filename, got, tt.size)
			}
		})
	}
}

func TestCollector_ParseJS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(TestingHandleRequest))
	// Close the server when test finishes
	defer server.Close()

	tests := []struct {
		filename string
		key      string
		want     string
	}{
		{
			filename: "system_data.js",
			key:      "model_name",
			want:     "GS1200-8",
		},
		{
			filename: "link_data.js",
			key:      "portstatus",
			want:     "Up,Down,Down,Down,Up,Down,Down,Up",
		},
		{
			filename: "VLAN_1Q_List_data.js",
			key:      "port_nums",
			want:     "8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			c := &Collector{
				address:  strings.Replace(server.URL, "http://", "", 1),
				password: "secret",
			}
			js, err := c.FetchJS(tt.filename)
			if err != nil {
				t.Errorf("Collector.ParseJS(%v) error = %v", tt.filename, err)
				return
			}
			vm = otto.New()
			if err = c.ParseJS(js); err != nil {
				t.Errorf("Collector.ParseJS(%v) error = %v", tt.filename, err)
				return
			}
			got, err := vm.Get(tt.key)
			if err != nil {
				t.Errorf("Collector.ParseJS(%v) error = %v", tt.filename, err)
				return
			}
			if got.String() != tt.want {
				t.Errorf("Collector.ParseJS() error = failed key %v: %v, want %v", tt.key, got.String(), tt.want)
			}
		})
	}
}
