from fastapi import FastAPI
from pydantic import BaseModel, Field


class RiskSummaryRequest(BaseModel):
    case_id: str = Field(min_length=1)
    merchant_id: str = Field(min_length=1)
    risk_category: str = Field(min_length=1)
    evidence_items: list[str] = Field(min_length=1)
    operator_note: str = ""


class RiskSummaryResponse(BaseModel):
    case_id: str
    summary: str
    grounded: bool
    suggestions: list[str]


def create_app() -> FastAPI:
    app = FastAPI(title="TrustOps AI Copilot", version="0.1.0")

    @app.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok", "service": "ai-copilot"}

    @app.post("/copilot/risk/summary", response_model=RiskSummaryResponse)
    def summarize_risk_case(payload: RiskSummaryRequest) -> RiskSummaryResponse:
        summary = (
            f"Case {payload.case_id} for merchant {payload.merchant_id} "
            f"is categorized as {payload.risk_category}. "
            f"Observed evidence: {', '.join(payload.evidence_items)}."
        )
        suggestions = [
            "Freeze high-risk listing actions until manual review completes.",
            "Cross-check merchant history with recent anomaly windows.",
            "Escalate to RiskOps if similar signals repeat in 24 hours.",
        ]
        return RiskSummaryResponse(
            case_id=payload.case_id,
            summary=summary,
            grounded=True,
            suggestions=suggestions,
        )

    return app


app = create_app()
