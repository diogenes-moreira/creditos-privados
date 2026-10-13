import React, { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Alert,
  AlertTitle,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Grid,
  InputAdornment,
  LinearProgress,
  MenuItem,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@mui/material";
import { Search as SearchIcon } from "@mui/icons-material";
import { adminAssessLivestockQuotaFromRegistry } from "../../api/endpoints";
import { getErrorMessage } from "../../api/errorUtils";
import type {
  LivestockQuotaAssessment,
  LivestockRegistryIdType,
  RegistryQuotaResponse,
} from "../../api/types";

/** Identification documents the registry accepts; the cédula is the default. */
const ID_TYPES: { value: LivestockRegistryIdType; key: string }[] = [
  { value: "CÉDULA DE CIUDADANÍA", key: "cedula" },
  { value: "CÉDULA DE EXTRANJERÍA", key: "extranjeria" },
  { value: "NIT", key: "nit" },
  { value: "PASAPORTE", key: "pasaporte" },
  { value: "LIBRETA MILITAR", key: "libretaMilitar" },
  { value: "TARJETA DE IDENTIDAD O REGISTRO CIVIL", key: "tarjetaIdentidad" },
];

/**
 * The registry is Colombian and so is the money it prices, so amounts are
 * grouped the Colombian way whatever locale the browser reports.
 */
const MONEY_LOCALE = "es-CO";

const parseDecimal = (raw: string): number => {
  const parsed = Number.parseFloat(raw);
  return Number.isFinite(parsed) ? parsed : NaN;
};

/**
 * Formats a decimal string for display. The string the backend sent is the
 * value of record: it is parsed to be printed, never to be recomputed.
 */
const formatMoney = (raw: string, currency: string): string => {
  const value = parseDecimal(raw);
  if (Number.isNaN(value)) return raw;
  return new Intl.NumberFormat(MONEY_LOCALE, {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
};

const formatDecimal = (raw: string, digits = 2): string => {
  const value = parseDecimal(raw);
  if (Number.isNaN(value)) return raw;
  return new Intl.NumberFormat(MONEY_LOCALE, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value);
};

const formatInteger = (value: number): string =>
  new Intl.NumberFormat(MONEY_LOCALE).format(value);

const CEILING_KEYS = ["ltv", "capacity", "inputs", "policyCap"] as const;
type CeilingKey = (typeof CEILING_KEYS)[number];

/**
 * Which of the four ceilings the quota was cut by.
 *
 * The backend sends a stable code for exactly this. The fallback compares the
 * ceiling values against the quota, which keeps older responses working and
 * costs nothing.
 */
const bindingCeilingOf = (assessment: LivestockQuotaAssessment): CeilingKey | null => {
  const code = assessment.activeCeilingCode?.trim() as CeilingKey | undefined;
  if (code && CEILING_KEYS.includes(code)) return code;

  const quota = parseDecimal(assessment.quota);
  if (Number.isNaN(quota)) return null;
  return (
    CEILING_KEYS.find((key) => {
      const value = parseDecimal(assessment.ceilings[key]);
      return !Number.isNaN(value) && Math.abs(value - quota) < 0.005;
    }) ?? null
  );
};

type ScoreColor = "success" | "info" | "warning" | "error";

const scoreColor = (score: number): ScoreColor => {
  if (score >= 80) return "success";
  if (score >= 60) return "info";
  if (score >= 50) return "warning";
  return "error";
};

const httpStatusOf = (error: unknown): number | undefined =>
  (error as { response?: { status?: number } })?.response?.status;

/**
 * Looks a rancher's pre-approved quota up in Fedegán's vaccination registry.
 *
 * The screen is written for someone who has to justify the number: the quota
 * never appears without the ceiling that bound it, the score's components and
 * the assumptions the registry forced on the assessment.
 */
const LivestockQuotaLookup: React.FC = () => {
  const { t } = useTranslation();

  const [nationalId, setNationalId] = useState("");
  const [idType, setIdType] = useState<LivestockRegistryIdType>("CÉDULA DE CIUDADANÍA");
  const [submitted, setSubmitted] = useState<{
    nationalId: string;
    idType: LivestockRegistryIdType;
  } | null>(null);

  const {
    data,
    error,
    isFetching,
  } = useQuery<RegistryQuotaResponse>({
    queryKey: ["livestock-quota-lookup", submitted?.nationalId, submitted?.idType],
    queryFn: () =>
      adminAssessLivestockQuotaFromRegistry(submitted!.nationalId, submitted!.idType),
    enabled: !!submitted,
    // A national ID missing from the registry is an answer, not a transient
    // failure: retrying it only delays the answer.
    retry: false,
    refetchOnWindowFocus: false,
  });

  const assessment = data?.assessment;
  const binding = useMemo(
    () => (assessment ? bindingCeilingOf(assessment) : null),
    [assessment]
  );

  const search = () => {
    const trimmed = nationalId.trim();
    if (!trimmed) return;
    setSubmitted({ nationalId: trimmed, idType });
  };

  const assumptionText = (code: string): string =>
    t(`livestock.lookup.assumption.${code}`, { defaultValue: code });

  const renderError = () => {
    const status = httpStatusOf(error);
    if (status === 404) {
      // Most of the target population has no RUV record. That is ordinary.
      return (
        <Alert severity="info">
          <AlertTitle>{t("livestock.lookup.notFoundTitle")}</AlertTitle>
          {t("livestock.lookup.notFoundBody", { nationalId: submitted?.nationalId })}
        </Alert>
      );
    }
    if (status === 503) {
      return (
        <Alert severity="warning">
          <AlertTitle>{t("livestock.lookup.notConfiguredTitle")}</AlertTitle>
          {t("livestock.lookup.notConfiguredBody")}
        </Alert>
      );
    }
    if (status === 502) {
      return (
        <Alert severity="error">
          <AlertTitle>{t("livestock.lookup.unreachableTitle")}</AlertTitle>
          {t("livestock.lookup.unreachableBody")}
        </Alert>
      );
    }
    if (status === 400) {
      return (
        <Alert severity="error">
          <AlertTitle>{t("livestock.lookup.invalidTitle")}</AlertTitle>
          {getErrorMessage(error, t("livestock.lookup.invalidBody"))}
        </Alert>
      );
    }
    return (
      <Alert severity="error">
        <AlertTitle>{t("livestock.lookup.failedTitle")}</AlertTitle>
        {getErrorMessage(error, t("livestock.lookup.failedBody"))}
      </Alert>
    );
  };

  const field = (label: string, value: React.ReactNode, hint?: string) => (
    <Box>
      <Typography variant="caption" color="text.secondary" display="block">
        {label}
      </Typography>
      <Typography variant="subtitle1" fontWeight={700}>
        {value}
      </Typography>
      {hint && (
        <Typography variant="caption" color="text.secondary">
          {hint}
        </Typography>
      )}
    </Box>
  );

  return (
    <Box>
      <Box mb={3}>
        <Typography variant="h4" fontWeight={700} gutterBottom>
          {t("livestock.lookup.title")}
        </Typography>
        <Typography color="text.secondary">{t("livestock.lookup.subtitle")}</Typography>
      </Box>

      <Paper sx={{ p: 2, mb: 3 }}>
        <Grid container spacing={2} alignItems="center">
          <Grid item xs={12} md={5}>
            <TextField
              fullWidth
              size="small"
              label={t("livestock.lookup.idLabel")}
              placeholder={t("livestock.lookup.idPlaceholder")}
              value={nationalId}
              onChange={(e) => setNationalId(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") search();
              }}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchIcon />
                  </InputAdornment>
                ),
              }}
            />
          </Grid>
          <Grid item xs={12} md={5}>
            <TextField
              select
              fullWidth
              size="small"
              label={t("livestock.lookup.idType")}
              value={idType}
              onChange={(e) => setIdType(e.target.value as LivestockRegistryIdType)}
            >
              {ID_TYPES.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {t(`livestock.lookup.idTypes.${option.key}`)}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid item xs={12} md={2}>
            <Button
              fullWidth
              variant="contained"
              startIcon={<SearchIcon />}
              disabled={!nationalId.trim() || isFetching}
              onClick={search}
            >
              {isFetching ? t("livestock.lookup.searching") : t("livestock.lookup.search")}
            </Button>
          </Grid>
        </Grid>
      </Paper>

      {!submitted && (
        <Alert severity="info">
          <AlertTitle>{t("livestock.lookup.emptyTitle")}</AlertTitle>
          {t("livestock.lookup.emptyHelp")}
        </Alert>
      )}

      {submitted && isFetching && (
        <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
          <CircularProgress />
        </Box>
      )}

      {submitted && !isFetching && error && renderError()}

      {submitted && !isFetching && !error && data && assessment && (
        <Stack spacing={3}>
          {/* The number, and immediately the reason it is that number. */}
          <Card>
            <CardContent>
              <Grid container spacing={2} alignItems="center">
                <Grid item xs={12} md={7}>
                  <Typography variant="overline" color="text.secondary">
                    {t("livestock.lookup.quotaTitle")}
                  </Typography>
                  <Typography variant="h3" fontWeight={800} color="primary.main">
                    {formatMoney(assessment.quota, assessment.currency)}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {t("livestock.lookup.quotaCaption", { currency: assessment.currency })}
                  </Typography>
                </Grid>
                <Grid item xs={12} md={5}>
                  <Stack spacing={1} alignItems={{ xs: "flex-start", md: "flex-end" }}>
                    <Chip
                      color="primary"
                      label={t("livestock.lookup.boundBy", {
                        ceiling: binding
                          ? t(`livestock.lookup.ceiling.${binding}`)
                          : assessment.activeCeiling,
                      })}
                    />
                    <Typography variant="caption" color="text.secondary">
                      {t("livestock.lookup.reference", { reference: assessment.reference })}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {t("livestock.lookup.assessedAt", {
                        date: new Date(assessment.createdAt).toLocaleString(),
                      })}
                    </Typography>
                  </Stack>
                </Grid>
              </Grid>
            </CardContent>
          </Card>

          {/* Not a footnote: 45 of the score's 100 points rest on these. */}
          {data.assumptions.length > 0 && (
            <Alert severity="warning">
              <AlertTitle>{t("livestock.lookup.assumptionsTitle")}</AlertTitle>
              <Typography variant="body2" sx={{ mb: 1 }}>
                {t("livestock.lookup.assumptionsIntro")}
              </Typography>
              <Box component="ul" sx={{ pl: 2.5, m: 0 }}>
                {data.assumptions.map((code) => (
                  <li key={code}>
                    <Typography variant="body2">{assumptionText(code)}</Typography>
                  </li>
                ))}
              </Box>
              <Typography variant="body2" sx={{ mt: 1, fontWeight: 700 }}>
                {t("livestock.lookup.assumptionsImpact")}
              </Typography>
              <Typography variant="body2" sx={{ mt: 1 }}>
                {t("livestock.lookup.assumptionsObserved")}
              </Typography>
            </Alert>
          )}

          <Card>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t("livestock.lookup.ceilingsTitle")}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("livestock.lookup.ceilingsHelp")}
              </Typography>
              <Grid container spacing={2}>
                {CEILING_KEYS.map((key) => {
                  const isBinding = binding === key;
                  const value = parseDecimal(assessment.ceilings[key]);
                  // A zero inputs ceiling that did not bind means the policy
                  // has the inputs ceiling switched off, not a quota of zero.
                  const notApplied = key === "inputs" && !isBinding && value === 0;
                  return (
                    <Grid item xs={12} sm={6} md={3} key={key}>
                      <Paper
                        variant="outlined"
                        sx={{
                          p: 2,
                          height: "100%",
                          borderColor: isBinding ? "primary.main" : "divider",
                          borderWidth: isBinding ? 2 : 1,
                          backgroundColor: isBinding ? "action.hover" : "background.paper",
                        }}
                      >
                        <Stack spacing={1}>
                          <Typography variant="subtitle2" color="text.secondary">
                            {t(`livestock.lookup.ceiling.${key}`)}
                          </Typography>
                          <Typography
                            variant="h6"
                            fontWeight={700}
                            color={isBinding ? "primary.main" : "text.primary"}
                          >
                            {notApplied
                              ? t("livestock.lookup.ceilingNotApplied")
                              : formatMoney(assessment.ceilings[key], assessment.currency)}
                          </Typography>
                          {isBinding && (
                            <Chip
                              size="small"
                              color="primary"
                              label={t("livestock.lookup.ceilingBinding")}
                            />
                          )}
                          <Typography variant="caption" color="text.secondary">
                            {notApplied
                              ? t("livestock.lookup.ceilingNotAppliedHint")
                              : t(`livestock.lookup.ceilingHelp.${key}`)}
                          </Typography>
                        </Stack>
                      </Paper>
                    </Grid>
                  );
                })}
              </Grid>
            </CardContent>
          </Card>

          <Card>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t("livestock.lookup.scoreTitle")}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("livestock.lookup.scoreHelp")}
              </Typography>
              <Grid container spacing={3} alignItems="center">
                <Grid item xs={12} md={4}>
                  <Stack direction="row" spacing={1} alignItems="baseline">
                    <Typography variant="h4" fontWeight={800}>
                      {assessment.score.score}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      {t("livestock.lookup.scoreOutOf")}
                    </Typography>
                    <Chip
                      size="small"
                      color={scoreColor(assessment.score.score)}
                      label={t("livestock.lookup.grade", { grade: assessment.score.grade })}
                    />
                  </Stack>
                  <LinearProgress
                    variant="determinate"
                    value={Math.min(100, Math.max(0, assessment.score.score))}
                    color={scoreColor(assessment.score.score)}
                    sx={{ mt: 1, height: 8, borderRadius: 4 }}
                  />
                </Grid>
                <Grid item xs={12} sm={4} md={3}>
                  {field(
                    t("livestock.lookup.vaccinationRate"),
                    `${assessment.score.vaccinationRatePct} %`
                  )}
                </Grid>
                <Grid item xs={12} sm={4} md={3}>
                  {field(
                    t("livestock.lookup.cycleCompliance"),
                    `${assessment.score.cycleCompliancePct} %`,
                    t("livestock.lookup.cycleComplianceHint")
                  )}
                </Grid>
                <Grid item xs={12} sm={4} md={2}>
                  {field(
                    t("livestock.lookup.seniority"),
                    t("livestock.lookup.seniorityValue", {
                      count: assessment.score.seniorityCycles,
                    })
                  )}
                </Grid>
              </Grid>
            </CardContent>
          </Card>

          <Card>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t("livestock.lookup.detailTitle")}
              </Typography>
              <Grid container spacing={2} sx={{ mt: 0.5 }}>
                <Grid item xs={6} sm={4} md={3}>
                  {field(t("livestock.lookup.typology"), assessment.typology)}
                </Grid>
                <Grid item xs={6} sm={4} md={3}>
                  {field(t("livestock.lookup.totalHead"), formatInteger(assessment.head))}
                </Grid>
                <Grid item xs={6} sm={4} md={3}>
                  {field(
                    t("livestock.lookup.ugg"),
                    formatDecimal(assessment.ugg),
                    t("livestock.lookup.uggHint")
                  )}
                </Grid>
                <Grid item xs={6} sm={4} md={3}>
                  {field(
                    t("livestock.lookup.projectedIncome"),
                    formatMoney(assessment.projectedIncome, assessment.currency)
                  )}
                </Grid>
                <Grid item xs={6} sm={4} md={3}>
                  {field(
                    t("livestock.lookup.eligibleBasket"),
                    formatMoney(assessment.eligibleBasket, assessment.currency),
                    t("livestock.lookup.eligibleBasketHint")
                  )}
                </Grid>
                <Grid item xs={6} sm={4} md={3}>
                  {field(
                    t("livestock.lookup.termMonths"),
                    t("livestock.lookup.termValue", { count: assessment.termMonths })
                  )}
                </Grid>
                <Grid item xs={6} sm={4} md={3}>
                  {field(
                    t("livestock.lookup.periodRate"),
                    `${formatDecimal(assessment.periodRatePct, 4)} %`,
                    t("livestock.lookup.periodRateHint")
                  )}
                </Grid>
              </Grid>
            </CardContent>
          </Card>

          <Card>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t("livestock.lookup.rancherTitle")}
              </Typography>
              <Grid container spacing={2} sx={{ mb: 2 }}>
                <Grid item xs={12} sm={6} md={3}>
                  {field(t("livestock.lookup.name"), data.rancher.name || "-")}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {field(t("livestock.lookup.nationalId"), data.rancher.nationalId)}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {field(
                    t("livestock.lookup.phone"),
                    data.rancher.phone || t("livestock.lookup.notReported")
                  )}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {field(
                    t("livestock.lookup.email"),
                    data.rancher.email || t("livestock.lookup.notReported")
                  )}
                </Grid>
              </Grid>

              <Divider sx={{ mb: 2 }} />

              <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                {t("livestock.lookup.holdingsTitle")}
              </Typography>
              {data.rancher.holdings.length === 0 ? (
                <Alert severity="info">{t("livestock.lookup.noHoldings")}</Alert>
              ) : (
                <Paper variant="outlined" sx={{ overflowX: "auto" }}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>{t("livestock.lookup.holding")}</TableCell>
                        <TableCell>{t("livestock.lookup.location")}</TableCell>
                        <TableCell>{t("livestock.lookup.cycle")}</TableCell>
                        <TableCell align="right">{t("livestock.lookup.head")}</TableCell>
                        <TableCell align="right">
                          {t("livestock.lookup.notVaccinated")}
                        </TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {data.rancher.holdings.map((holding) => (
                        <TableRow key={`${holding.code}-${holding.year}-${holding.cycle}`}>
                          <TableCell>
                            <Typography variant="body2" fontWeight={600}>
                              {holding.name || holding.code}
                            </Typography>
                            <Typography variant="caption" color="text.secondary">
                              {holding.code}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2">
                              {[holding.municipality, holding.department]
                                .filter(Boolean)
                                .join(", ")}
                            </Typography>
                            {holding.village && (
                              <Typography variant="caption" color="text.secondary">
                                {holding.village}
                              </Typography>
                            )}
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2">
                              {t("livestock.lookup.cycleValue", {
                                cycle: holding.cycle,
                                year: holding.year,
                              })}
                            </Typography>
                            {holding.vaccinatedAt && (
                              <Typography variant="caption" color="text.secondary">
                                {t("livestock.lookup.vaccinatedAt", {
                                  date: holding.vaccinatedAt,
                                })}
                              </Typography>
                            )}
                          </TableCell>
                          <TableCell align="right">{formatInteger(holding.head)}</TableCell>
                          <TableCell align="right">
                            {holding.notVaccinated > 0 ? (
                              <Chip
                                size="small"
                                color="warning"
                                label={formatInteger(holding.notVaccinated)}
                              />
                            ) : (
                              formatInteger(holding.notVaccinated)
                            )}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </Paper>
              )}
            </CardContent>
          </Card>
        </Stack>
      )}
    </Box>
  );
};

export default LivestockQuotaLookup;
