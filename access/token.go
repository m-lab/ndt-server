package access

import (
	"context"
	"flag"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gopkg.in/square/go-jose.v2/jwt"
)

// TokenController manages access control for clients providing access_token parameters.
type TokenController struct {
	token   Verifier
	machine string
}

const monitorIssuer = "monitoring"

var (
	tokenAccessRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_access_tokencontroller_requests_total",
			Help: "Total number of requests handled by the access tokencontroller.",
		},
		[]string{"request"},
	)
	requireTokens bool
)

func init() {
	flag.BoolVar(&requireTokens, "tokencontroller.required", false, "Whether access tokens are required by HTTP-based clients.")
}

// Verifier is used by the TokenController to verify JWT claims in access tokens.
type Verifier interface {
	Verify(token string, exp jwt.Expected) (*jwt.Claims, error)
}

// NewTokenController creates a new token controller.
func NewTokenController(name string, verifier Verifier) *TokenController {
	return &TokenController{
		token:   verifier,
		machine: name,
	}
}

// Limit implements the Controller interface by checking clients provided access_tokens.
func (t *TokenController) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verified, ctx := t.isVerified(r)
		if !verified {
			// 403 - https://tools.ietf.org/html/rfc7231#section-6.5.3
			w.WriteHeader(http.StatusUnauthorized)
			// Return without additional response.
			return
		}
		// Clone the request with the context provided by isVerified.
		next.ServeHTTP(w, r.Clone(ctx))
	})
}

// isVerified validates the access_token and if the access token issuer is
// monitoring, add a context value derived from the given request context.
func (t *TokenController) isVerified(r *http.Request) (bool, context.Context) {
	ctx := r.Context()
	token := r.Form.Get("access_token")
	if token == "" && !requireTokens {
		// TODO: after migrating clients to locate service, require access_token.
		// For now, accept the request.
		tokenAccessRequests.WithLabelValues("accepted").Inc()
		return true, ctx
	}
	// Attempt to verify the token.
	cl, err := t.token.Verify(token, jwt.Expected{
		// Do not specify the Issuer here so we can check for monitoring or the
		// locate service below.
		Subject:  "ndt",
		Audience: jwt.Audience{t.machine}, // current server.
		Time:     time.Now(),
	})
	if err != nil {
		// The access token was invalid; reject this request.
		tokenAccessRequests.WithLabelValues("rejected").Inc()
		return false, ctx
	}
	// If the claim was for monitoring, set the context value so subsequent access
	// controllers can check the advisory information to exepmpt the request.
	tokenAccessRequests.WithLabelValues("accepted").Inc()
	return true, SetMonitoring(ctx, cl.Issuer == monitorIssuer)
}
