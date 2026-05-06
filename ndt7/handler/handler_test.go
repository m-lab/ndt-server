// Package handler implements the WebSocket handler for ndt7.
package handler

import (
	"context"
	"net/url"
	"reflect"
	"testing"

	"github.com/m-lab/access/controller"
	"github.com/m-lab/ndt-server/ndt7/download/sender"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func TestAppendIntegrationMetadata(t *testing.T) {
	tests := []struct {
		name      string
		claim     any
		wantLen   int
		wantIntID string
		wantKeyID string
	}{
		{
			name:    "nil-claims",
			claim:   nil,
			wantLen: 0,
		},
		{
			name:    "empty-claims",
			claim:   &IntegrationClaims{},
			wantLen: 0,
		},
		{
			name:      "with-both-claims",
			claim:     &IntegrationClaims{IntegrationID: "test-int", KeyID: "ki_test"},
			wantLen:   2,
			wantIntID: "test-int",
			wantKeyID: "ki_test",
		},
		{
			name:      "with-int-id-only",
			claim:     &IntegrationClaims{IntegrationID: "test-int"},
			wantLen:   1,
			wantIntID: "test-int",
		},
		{
			name:      "with-key-id-only",
			claim:     &IntegrationClaims{KeyID: "ki_test"},
			wantLen:   1,
			wantKeyID: "ki_test",
		},
		{
			name:    "unexpected-claim-type",
			claim:   &struct{ Foo string }{Foo: "bar"},
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &model.ArchivalData{}
			ctx := context.Background()
			if tt.claim != nil {
				ctx = controller.SetCustomClaim(ctx, tt.claim)
			}
			appendIntegrationMetadata(ctx, data)
			if len(data.ClientMetadata) != tt.wantLen {
				t.Errorf("Expected %d metadata entries, got %d: %+v", tt.wantLen, len(data.ClientMetadata), data.ClientMetadata)
			}
			for _, nv := range data.ClientMetadata {
				switch nv.Name {
				case "int_id":
					if nv.Value != tt.wantIntID {
						t.Errorf("Expected int_id %q, got %q", tt.wantIntID, nv.Value)
					}
				case "key_id":
					if nv.Value != tt.wantKeyID {
						t.Errorf("Expected key_id %q, got %q", tt.wantKeyID, nv.Value)
					}
				default:
					t.Errorf("Unexpected metadata entry: %+v", nv)
				}
			}
		})
	}
}

func TestAppendClientMetadata_FiltersIntegrationKeys(t *testing.T) {
	data := &model.ArchivalData{}
	values := url.Values{
		"client_name": {"ndt7-client"},
		"int_id":      {"spoofed-int"},
		"key_id":      {"spoofed-key"},
		"server_foo":  {"bar"},
	}
	appendClientMetadata(data, values)
	for _, nv := range data.ClientMetadata {
		switch nv.Name {
		case "int_id", "key_id":
			t.Errorf("int_id/key_id should be filtered from query string, got %+v", nv)
		case "server_foo":
			t.Errorf("server_ keys should be filtered, got %+v", nv)
		}
	}
	if len(data.ClientMetadata) != 1 || data.ClientMetadata[0].Name != "client_name" {
		t.Errorf("Expected only client_name, got %+v", data.ClientMetadata)
	}
}

func Test_validateEarlyExit(t *testing.T) {
	tests := []struct {
		name    string
		values  url.Values
		want    *sender.Params
		wantErr bool
	}{
		{
			name:   "valid-param",
			values: url.Values{"early_exit": {spec.ValidEarlyExitValues[0]}},
			want: &sender.Params{
				IsEarlyExit: true,
				MaxBytes:    250000000,
			},
			wantErr: false,
		},
		{
			name:    "invalid-param",
			values:  url.Values{"early_exit": {"123"}},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing-value",
			values:  url.Values{"early_exit": {""}},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "absent-param",
			values: url.Values{"foo": {"bar"}},
			want: &sender.Params{
				IsEarlyExit: false,
				MaxBytes:    0,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateEarlyExit(tt.values)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEarlyExit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateEarlyExit() = %v, want %v", got, tt.want)
			}
		})
	}
}
