import React from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertTitle, Box, Card, CardContent, Chip, Grid, Stack, Typography } from "@mui/material";
import type { LivestockQuotaAssessment, QuotaTier, QuotaTierReason } from "../api/types";

const formatAmount = (raw: string | undefined, currency: string): string => {
  const value = Number.parseFloat(raw ?? "");
  if (!Number.isFinite(value)) return raw ?? "";
  return new Intl.NumberFormat("es-CO", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(value);
};

const formatRate = (raw: string): string => {
  const value = Number.parseFloat(raw);
  if (!Number.isFinite(value)) return raw;
  return `${(value * 100).toFixed(2)}%`;
};

/**
 * The pilot tier decision (500k, 1M, 2M COP): the tier granted, the figures it
 * was measured on, and what blocked the next tier or the entry filter.
 *
 * The engine's quota is shown next to it only for comparison: under the tiers
 * strategy the tier amount is what is offered.
 */
const QuotaTierCard: React.FC<{ assessment: LivestockQuotaAssessment; tier: QuotaTier }> = ({
  assessment,
  tier,
}) => {
  const { t } = useTranslation();
  const eligible = tier.code !== "notEligible";
  const entryFailed = tier.reasons.some((r) => r.scope === "entry");
  const strategy = assessment.strategy ?? "tiers";

  // The rate travels as a fraction; show it as a percentage like the label.
  const shown = (r: QuotaTierReason, value: string): string =>
    r.requirement === "vaccinationRate" ? value.replace(/[0-9.]+/, (n) => formatRate(n)) : value;

  const reasonText = (r: QuotaTierReason): string =>
    t("livestock.tier.reasonLine", {
      requirement: t(`livestock.tier.requirement.${r.requirement}`, { defaultValue: r.requirement }),
      required: shown(r, r.required),
      actual: shown(r, r.actual),
    });

  const measured: { label: string; value: string; hint?: string }[] = [
    {
      label: t("livestock.tier.vaccinatedHeads", { cycle: tier.currentCycle }),
      value: String(tier.vaccinatedHeads),
    },
    { label: t("livestock.tier.vaccinationRate"), value: formatRate(tier.vaccinationRate) },
    {
      label: t("livestock.tier.currentCycleFound", { cycle: tier.currentCycle }),
      value: tier.currentCycleFound ? t("livestock.tier.yes") : t("livestock.tier.no"),
    },
    {
      label: t("livestock.tier.ownedHolding"),
      value: tier.ownedHolding ? t("livestock.tier.yes") : t("livestock.tier.no"),
      hint: tier.ownedHolding ? undefined : t("livestock.tier.ownedHoldingHint"),
    },
  ];

  return (
    <Card>
      <CardContent>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap sx={{ mb: 2 }}>
          <Typography variant="h6" sx={{ mr: 1 }}>
            {t("livestock.tier.title")}
          </Typography>
          <Chip
            size="small"
            variant="outlined"
            label={t("livestock.tier.strategy", {
              strategy: t(`livestock.tier.strategyName.${strategy}`),
            })}
          />
          <Chip size="small" variant="outlined" label={t("livestock.tier.policyVersion", { version: tier.policyVersion })} />
        </Stack>

        <Grid container spacing={3} sx={{ mb: 2 }}>
          <Grid item xs={12} sm={6}>
            <Typography variant="overline" color="text.secondary">
              {t("livestock.tier.assigned")}
            </Typography>
            <Typography variant="h4" fontWeight={800} color={eligible ? "primary" : "text.secondary"}>
              {eligible ? formatAmount(tier.amount, assessment.currency) : t("livestock.tier.notEligible")}
            </Typography>
            <Chip
              size="small"
              color={eligible ? "success" : "default"}
              label={eligible ? tier.label : t("livestock.tier.notEligible")}
            />
          </Grid>
          {assessment.engineQuota !== undefined && (
            <Grid item xs={12} sm={6}>
              <Typography variant="overline" color="text.secondary">
                {t("livestock.tier.engineQuota")}
              </Typography>
              <Typography variant="h5" fontWeight={700} color="text.secondary">
                {formatAmount(assessment.engineQuota, assessment.currency)}
              </Typography>
            </Grid>
          )}
        </Grid>

        <Typography variant="subtitle2" gutterBottom>
          {t("livestock.tier.measuredTitle")}
        </Typography>
        <Grid container spacing={2} sx={{ mb: 2 }}>
          {measured.map((m) => (
            <Grid item xs={6} md={3} key={m.label}>
              <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                {m.label}
              </Typography>
              <Typography variant="body1" fontWeight={700}>
                {m.value}
              </Typography>
              {m.hint && (
                <Typography variant="caption" color="text.secondary">
                  {m.hint}
                </Typography>
              )}
            </Grid>
          ))}
        </Grid>

        {tier.reasons.length > 0 ? (
          <Alert severity={entryFailed ? "error" : "info"}>
            <AlertTitle>
              {entryFailed ? t("livestock.tier.reasonsTitleEntry") : t("livestock.tier.reasonsTitleNext")}
            </AlertTitle>
            <Box component="ul" sx={{ m: 0, pl: 2 }}>
              {tier.reasons.map((r) => (
                <li key={`${r.scope}-${r.requirement}`}>
                  <Typography variant="body2">
                    {!entryFailed && `${t(`livestock.tier.scope.${r.scope}`, { defaultValue: r.scope })} · `}
                    {reasonText(r)}
                  </Typography>
                </li>
              ))}
            </Box>
          </Alert>
        ) : (
          <Alert severity="success">{t("livestock.tier.topTier")}</Alert>
        )}

        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>
          {t("livestock.tier.outsideNote")}
        </Typography>
      </CardContent>
    </Card>
  );
};

export default QuotaTierCard;
