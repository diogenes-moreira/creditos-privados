import React from "react";
import { useTranslation } from "react-i18next";
import {
  Box,
  Card,
  CardContent,
  Chip,
  Divider,
  Grid,
  LinearProgress,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  TrendingUp as PositiveIcon,
  TrendingDown as NegativeIcon,
  Remove as NeutralIcon,
  InfoOutlined as InfoIcon,
} from "@mui/icons-material";
import type { FactorImpact, RiskAssessment, RiskDecision, RiskEvaluation } from "../api/types";
import MoneyDisplay from "./MoneyDisplay";

const decisionColor: Record<RiskDecision, "success" | "warning" | "error"> = {
  approve: "success",
  manual_review: "warning",
  decline: "error",
};

const gradeColor = (grade: string): "success" | "info" | "warning" | "error" => {
  switch (grade) {
    case "A":
      return "success";
    case "B":
      return "info";
    case "C":
      return "warning";
    default:
      return "error";
  }
};

const impactIcon = (impact: FactorImpact) => {
  switch (impact) {
    case "positive":
      return <PositiveIcon fontSize="small" color="success" />;
    case "negative":
      return <NegativeIcon fontSize="small" color="error" />;
    default:
      return <NeutralIcon fontSize="small" color="disabled" />;
  }
};

interface RiskAssessmentCardProps {
  /** A stored assessment, or an unsaved evaluation from a simulation. */
  assessment?: RiskAssessment;
  evaluation?: RiskEvaluation;
  title?: string;
}

const RiskAssessmentCard: React.FC<RiskAssessmentCardProps> = ({ assessment, evaluation, title }) => {
  const { t } = useTranslation();
  const result = assessment ?? evaluation;
  if (!result) return null;

  const percent = (raw: string) => `${(Number(raw) * 100).toFixed(1)}%`;

  return (
    <Card variant="outlined">
      <CardContent>
        {title && (
          <Typography variant="subtitle1" fontWeight={600} gutterBottom>
            {title}
          </Typography>
        )}

        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap sx={{ mb: 2 }}>
          <Chip
            label={t(`risk.decision.${result.decision}`)}
            color={decisionColor[result.decision]}
            sx={{ fontWeight: 600 }}
          />
          <Chip
            label={t("risk.gradeLabel", { grade: result.grade })}
            color={gradeColor(result.grade)}
            variant="outlined"
          />
          {assessment && (
            <Chip
              size="small"
              variant="outlined"
              label={t("risk.modelVersion", {
                name: assessment.riskModelName,
                version: assessment.modelVersion,
              })}
            />
          )}
          <Tooltip title={t("risk.analyzerHint")}>
            <Chip size="small" variant="outlined" label={result.analyzer} icon={<InfoIcon />} />
          </Tooltip>
        </Stack>

        <Box sx={{ mb: 2 }}>
          <Box sx={{ display: "flex", justifyContent: "space-between", mb: 0.5 }}>
            <Typography variant="body2" color="text.secondary">
              {t("risk.score")}
            </Typography>
            <Typography variant="body2" fontWeight={600}>
              {result.score} / 100
            </Typography>
          </Box>
          <LinearProgress
            variant="determinate"
            value={result.score}
            color={gradeColor(result.grade)}
            sx={{ height: 8, borderRadius: 4 }}
          />
        </Box>

        <Grid container spacing={2}>
          <Grid item xs={6} sm={3}>
            <Typography variant="caption" color="text.secondary">
              {t("risk.recommendedLimit")}
            </Typography>
            <Typography variant="h6">
              <MoneyDisplay amount={result.recommendedLimit} variant="h6" fontWeight={600} />
            </Typography>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Typography variant="caption" color="text.secondary">
              {t("risk.maxMonthlyPayment")}
            </Typography>
            <Typography variant="h6">
              <MoneyDisplay amount={result.maxMonthlyPayment} variant="h6" />
            </Typography>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Typography variant="caption" color="text.secondary">
              {t("risk.recommendedTerm")}
            </Typography>
            <Typography variant="h6">
              {t("risk.months", { count: result.recommendedInstallments })}
            </Typography>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Typography variant="caption" color="text.secondary">
              {t("risk.recommendedRate")}
            </Typography>
            <Typography variant="h6">{percent(result.recommendedInterestRate)}</Typography>
          </Grid>
        </Grid>

        {result.summary && (
          <>
            <Divider sx={{ my: 2 }} />
            <Typography variant="body2">{result.summary}</Typography>
          </>
        )}

        {result.factors && result.factors.length > 0 && (
          <>
            <Divider sx={{ my: 2 }} />
            <Typography variant="subtitle2" gutterBottom>
              {t("risk.factors")}
            </Typography>
            <Stack spacing={1}>
              {result.factors.map((factor) => (
                <Box key={factor.code} sx={{ display: "flex", gap: 1, alignItems: "flex-start" }}>
                  {impactIcon(factor.impact)}
                  <Box sx={{ flex: 1 }}>
                    <Typography variant="body2" fontWeight={500}>
                      {factor.label}
                      {factor.value ? ` — ${factor.value}` : ""}
                    </Typography>
                    {factor.comment && (
                      <Typography variant="caption" color="text.secondary">
                        {factor.comment}
                      </Typography>
                    )}
                  </Box>
                  <Typography variant="body2" color="text.secondary">
                    {factor.points}
                  </Typography>
                </Box>
              ))}
            </Stack>
          </>
        )}

        {result.policyNotes && result.policyNotes.length > 0 && (
          <>
            <Divider sx={{ my: 2 }} />
            <Typography variant="subtitle2" gutterBottom>
              {t("risk.policyNotes")}
            </Typography>
            <Stack spacing={0.5}>
              {result.policyNotes.map((note, index) => (
                <Typography key={index} variant="caption" color="text.secondary">
                  • {note}
                </Typography>
              ))}
            </Stack>
          </>
        )}

        <Divider sx={{ my: 2 }} />
        <Typography variant="caption" color="text.secondary">
          {t("risk.probabilityOfDefault")}: {percent(result.probabilityOfDefault)}
          {assessment && ` · ${new Date(assessment.createdAt).toLocaleString()}`}
        </Typography>
      </CardContent>
    </Card>
  );
};

export default RiskAssessmentCard;
