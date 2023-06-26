// Package handler implements the WebSocket handler for ndt7.
package handler

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/m-lab/ndt-server/ndt7/spec"
)

func Test_validateEarlyExit(t *testing.T) {
	type args struct {
		values url.Values
	}
	tests := []struct {
		name    string
		values  url.Values
		want    *spec.Params
		wantErr bool
	}{
		{
			name:   "valid-param",
			values: url.Values{"early_exit": {spec.ValidEarlyExitValues[0]}},
			want: &spec.Params{
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
			want: &spec.Params{
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
