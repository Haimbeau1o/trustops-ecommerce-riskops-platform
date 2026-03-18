package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/mq"
	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/storage"
)

type ingestRiskEventRequest struct {
	MerchantID   string   `json:"merchant_id"`
	EventType    string   `json:"event_type"`
	Evidence     []string `json:"evidence"`
	RiskScore    float64  `json:"risk_score"`
	TriggeredBy  string   `json:"triggered_by,omitempty"`
	OccurredAtMs int64    `json:"occurred_at_ms,omitempty"`
}

// Dependencies are injectable runtime components for the router.
type Dependencies struct {
	Repository  storage.Repository
	Publisher   mq.Publisher
	IDGenerator func() string
	Clock       func() time.Time
}

// NewRouter builds the API surface for the risk-ops gateway.
func NewRouter(deps Dependencies) *server.Hertz {
	return NewRouterWithHostPort("0.0.0.0:8080", deps)
}

// NewRouterWithHostPort builds the API surface and binds to a provided host:port.
func NewRouterWithHostPort(hostPort string, deps Dependencies) *server.Hertz {
	resolved := resolveDependencies(deps)
	h := server.New(server.WithHostPorts(hostPort))

	h.GET("/healthz", func(_ context.Context, c *app.RequestContext) {
		c.JSON(200, utils.H{
			"status":  "ok",
			"service": "gateway-go",
		})
	})

	h.POST("/api/v1/risk/events/ingest", func(_ context.Context, c *app.RequestContext) {
		var req ingestRiskEventRequest
		if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
			c.JSON(400, utils.H{"error": "invalid_json"})
			return
		}
		if req.MerchantID == "" || req.EventType == "" || len(req.Evidence) == 0 {
			c.JSON(400, utils.H{"error": "missing_required_fields"})
			return
		}

		now := resolved.Clock()
		occurredAtMs := req.OccurredAtMs
		if occurredAtMs == 0 {
			occurredAtMs = now.UnixMilli()
		}

		caseID := resolved.IDGenerator()
		riskCase := storage.Case{
			CaseID:        caseID,
			MerchantID:    req.MerchantID,
			EventType:     req.EventType,
			RiskCategory:  req.EventType,
			CaseStatus:    "pending_review",
			EvidenceItems: req.Evidence,
			RiskScore:     req.RiskScore,
			TriggeredBy:   req.TriggeredBy,
			OccurredAtMs:  occurredAtMs,
			CreatedAt:     now.UTC(),
		}
		ctx := context.Background()
		if err := resolved.Repository.CreateCase(ctx, riskCase); err != nil {
			c.JSON(500, utils.H{"error": "case_persist_failed"})
			return
		}

		eventPublished := true
		if err := resolved.Publisher.PublishCaseIngested(ctx, mq.CaseIngestedEvent{
			CaseID:       caseID,
			MerchantID:   req.MerchantID,
			EventType:    req.EventType,
			RiskScore:    req.RiskScore,
			OccurredAtMs: occurredAtMs,
		}); err != nil {
			eventPublished = false
		}

		c.JSON(202, utils.H{
			"accepted":        true,
			"case_id":         caseID,
			"risk_score":      req.RiskScore,
			"merchant_id":     req.MerchantID,
			"event_published": eventPublished,
		})
	})

	h.GET("/api/v1/risk/cases/:case_id", func(_ context.Context, c *app.RequestContext) {
		caseID := c.Param("case_id")
		riskCase, err := resolved.Repository.GetCase(context.Background(), caseID)
		if err != nil {
			if errors.Is(err, storage.ErrCaseNotFound) {
				c.JSON(404, utils.H{"error": "case_not_found"})
				return
			}
			c.JSON(500, utils.H{"error": "case_query_failed"})
			return
		}

		riskCategory := riskCase.RiskCategory
		if riskCategory == "" {
			riskCategory = riskCase.EventType
		}
		caseStatus := riskCase.CaseStatus
		if caseStatus == "" {
			caseStatus = "pending_review"
		}

		c.JSON(200, utils.H{
			"case_id":        caseID,
			"merchant_id":    riskCase.MerchantID,
			"risk_category":  riskCategory,
			"case_status":    caseStatus,
			"evidence_items": riskCase.EvidenceItems,
			"risk_score":     riskCase.RiskScore,
		})
	})

	return h
}

func resolveDependencies(deps Dependencies) Dependencies {
	if deps.Repository == nil {
		deps.Repository = storage.NewInMemoryRepository()
	}
	if deps.Publisher == nil {
		deps.Publisher = mq.NewNoopPublisher()
	}
	if deps.IDGenerator == nil {
		deps.IDGenerator = func() string {
			return fmt.Sprintf("case-%d", time.Now().UnixNano())
		}
	}
	if deps.Clock == nil {
		deps.Clock = func() time.Time {
			return time.Now()
		}
	}
	return deps
}
