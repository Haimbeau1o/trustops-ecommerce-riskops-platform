package http

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

type ingestRiskEventRequest struct {
	MerchantID   string   `json:"merchant_id"`
	EventType    string   `json:"event_type"`
	Evidence     []string `json:"evidence"`
	RiskScore    float64  `json:"risk_score"`
	TriggeredBy  string   `json:"triggered_by,omitempty"`
	OccurredAtMs int64    `json:"occurred_at_ms,omitempty"`
}

// NewRouter builds the API surface for the risk-ops gateway.
func NewRouter() *server.Hertz {
	h := server.New(server.WithHostPorts("0.0.0.0:8080"))

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

		caseID := fmt.Sprintf("case-%d", time.Now().UnixNano())
		c.JSON(202, utils.H{
			"accepted":    true,
			"case_id":     caseID,
			"risk_score":  req.RiskScore,
			"merchant_id": req.MerchantID,
		})
	})

	h.GET("/api/v1/risk/cases/:case_id", func(_ context.Context, c *app.RequestContext) {
		caseID := c.Param("case_id")
		c.JSON(200, utils.H{
			"case_id":       caseID,
			"merchant_id":   "merchant-1001",
			"risk_category": "listing_fraud",
			"case_status":   "pending_review",
			"evidence_items": []string{
				"sku_spike",
				"ip_anomaly",
			},
		})
	})

	return h
}
