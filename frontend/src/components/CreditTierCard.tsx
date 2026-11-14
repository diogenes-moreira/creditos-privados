import React from "react";
import { useTranslation } from "react-i18next";
import {
  Alert,
  AlertTitle,
  Box,
  Card,
  CardContent,
  Chip,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  CheckCircle as MetIcon,
  Cancel as UnmetIcon,
  HelpOutline as UnverifiedIcon,
} from "@mui/icons-material";
import type { CreditTier, TierCheckStatus, TierCriterion } from "../api/types";

/** The rows of the pre-approval table, in the order the policy lists them. */
const ROWS = [
  "vaccinatedHeads",
  "reportedCycles",
  "vaccinationRate",
  "completeCycles",
  "ownedHolding",
] as const;

const formatAmount = (raw: string, currency: string): string => {
  const value = Number.parseFloat(raw);
  if (!Number.isFinite(value)) return raw;
  return new Intl.NumberFormat("es-CO", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(value);
};

const StatusIcon: React.FC<{ status: TierCheckStatus }> = ({ status }) => {
  if (status === "met") return <MetIcon fontSize="small" color="success" />;
  if (status === "unmet") return <UnmetIcon fontSize="small" color="error" />;
  return <UnverifiedIcon fontSize="small" color="warning" />;
};

/**
 * The pre-approval by layers (500k, 1M, 2M): the credit available, and for
 * every layer which thresholds the registry shows as met, failed or not yet
 * verifiable.
 *
 * Two amounts are kept apart on purpose. The confirmed one rests only on
 * verified facts; the provisional one is what the rancher reaches once the
 * unverified items are confirmed. Showing only the second would present a
 * guess as a pre-approval.
 */
const CreditTierCard: React.FC<{ tier: CreditTier }> = ({ tier }) => {
  const { t } = useTranslation();
  const criterion = (code: string) => t(`livestock.tier.criterion.${code}`, { defaultValue: code });
  const statusText = (s: TierCheckStatus) => t(`livestock.tier.status.${s}`);

  const cell = (c: TierCriterion | undefined) => {
    if (!c) {
      return (
        <Typography variant="body2" color="text.secondary">
          {t("livestock.tier.noMinimum")}
        </Typography>
      );
    }
    return (
      <Tooltip title={statusText(c.status)}>
        <Stack direction="row" spacing={0.75} alignItems="center">
          <StatusIcon status={c.status} />
          <Typography variant="body2">
            {c.required ?? ""}
            {c.actual && (
              <Typography component="span" variant="caption" color="text.secondary">
                {" "}
                ({c.status === "unverified" && c.code !== "ownedHolding"
                  ? t("livestock.tier.seen", { value: c.actual })
                  : t("livestock.tier.has", { value: c.actual })})
              </Typography>
            )}
          </Typography>
        </Stack>
      </Tooltip>
    );
  };

  const confirmed = tier.code !== "";
  const provisionalAbove = tier.provisionalCode !== "" && tier.provisionalCode !== tier.code;

  return (
    <Card>
      <CardContent>
        <Typography variant="h6" gutterBottom>
          {t("livestock.tier.title")}
        </Typography>

        <Stack direction={{ xs: "column", sm: "row" }} spacing={4} sx={{ mb: 2 }}>
          <Box>
            <Typography variant="overline" color="text.secondary">
              {t("livestock.tier.confirmed")}
            </Typography>
            <Typography variant="h4" fontWeight={800} color={confirmed ? "primary" : "text.secondary"}>
              {formatAmount(tier.amount, tier.currency)}
            </Typography>
            <Chip
              size="small"
              label={confirmed ? t("livestock.tier.layer", { code: tier.code }) : t("livestock.tier.noLayer")}
              color={confirmed ? "success" : "default"}
            />
          </Box>
          {provisionalAbove && (
            <Box>
              <Typography variant="overline" color="text.secondary">
                {t("livestock.tier.provisional")}
              </Typography>
              <Typography variant="h4" fontWeight={800} color="warning.main">
                {formatAmount(tier.provisionalAmount, tier.currency)}
              </Typography>
              <Chip
                size="small"
                variant="outlined"
                color="warning"
                label={t("livestock.tier.layer", { code: tier.provisionalCode })}
              />
            </Box>
          )}
        </Stack>

        {!tier.eligible && (
          <Alert severity="error" sx={{ mb: 2 }}>
            <AlertTitle>{t("livestock.tier.notEligibleTitle")}</AlertTitle>
            {tier.entry
              .filter((c) => c.status === "unmet")
              .map((c) => criterion(c.code))
              .join(" · ")}
          </Alert>
        )}

        {tier.eligible && (tier.pending.length > 0 || provisionalAbove) && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            <AlertTitle>{t("livestock.tier.pendingTitle")}</AlertTitle>
            <Box component="ul" sx={{ m: 0, pl: 2 }}>
              {tier.pending.map((code) => (
                <li key={code}>{criterion(code)}</li>
              ))}
              {!tier.vaccination.historyKnown && <li>{t("livestock.tier.historyMissing")}</li>}
              {provisionalAbove &&
                tier.tiers
                  .find((e) => e.code === tier.provisionalCode)
                  ?.criteria.some((c) => c.code === "ownedHolding" && c.status === "unverified") && (
                  <li>{criterion("ownedHolding")}</li>
                )}
            </Box>
          </Alert>
        )}

        <Paper variant="outlined" sx={{ overflowX: "auto" }}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>{t("livestock.tier.criterionHeader")}</TableCell>
                {tier.tiers.map((e) => (
                  <TableCell key={e.code}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Typography variant="body2" fontWeight={700}>
                        {e.code}
                      </Typography>
                      {e.qualifies && <MetIcon fontSize="small" color="success" />}
                    </Stack>
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              <TableRow>
                <TableCell>{t("livestock.tier.entryRow")}</TableCell>
                <TableCell colSpan={tier.tiers.length}>
                  <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap>
                    {tier.entry.map((c) => (
                      <Tooltip key={c.code} title={statusText(c.status)}>
                        <Stack direction="row" spacing={0.5} alignItems="center">
                          <StatusIcon status={c.status} />
                          <Typography variant="body2">{criterion(c.code)}</Typography>
                        </Stack>
                      </Tooltip>
                    ))}
                  </Stack>
                </TableCell>
              </TableRow>
              {ROWS.map((row) => (
                <TableRow key={row}>
                  <TableCell>{criterion(row)}</TableCell>
                  {tier.tiers.map((e) => (
                    <TableCell key={e.code}>{cell(e.criteria.find((c) => c.code === row))}</TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>

        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>
          {t("livestock.tier.latestCycle", {
            cycle: tier.vaccination.latestCycle,
            year: tier.vaccination.latestYear,
          })}
        </Typography>
      </CardContent>
    </Card>
  );
};

export default CreditTierCard;
