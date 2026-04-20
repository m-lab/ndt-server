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
		ic        *IntegrationClaims
		wantLen   int
		wantIntID string
		wantKeyID string
	}{
		{
			name:    "nil-claims",
			ic:      nil,
			wantLen: 0,
		},
		{
			name:    "empty-claims",
			ic:      &IntegrationClaims{},
			wantLen: 0,
		},
		{
			name:      "with-both-claims",
			ic:        &IntegrationClaims{IntegrationID: "test-int", KeyID: "ki_test"},
			wantLen:   2,
			wantIntID: "test-int",
			wantKeyID: "ki_test",
		},
		{
			name:      "with-int-id-only",
			ic:        &IntegrationClaims{IntegrationID: "test-int"},
			wantLen:   1,
			wantIntID: "test-int",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &model.ArchivalData{}
			ctx := context.Background()
			if tt.ic != nil {
				ctx = controller.SetCustomClaim(ctx, tt.ic)
			}
			appendIntegrationMetadata(data, ctx)
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

func Test_validateEarlyExit(t *testing.T) {
	type args struct {
		values url.Values
	}
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
