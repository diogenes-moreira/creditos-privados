import React, { useEffect, useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  FormControlLabel,
  Grid,
  MenuItem,
  Paper,
  Stack,
  Switch,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";
import {
  Save as SaveIcon,
  CheckCircle as ActivateIcon,
  RestartAlt as ResetIcon,
} from "@mui/icons-material";
import {
  adminActivateLivestockQuotaPolicy,
  adminGetActiveLivestockQuotaPolicy,
  adminGetLivestockQuotaPolicies,
  adminSaveLivestockQuotaPolicyVersion,
} from "../../api/endpoints";
import { getErrorMessage } from "../../api/errorUtils";
import { useNotification } from "../../contexts/NotificationContext";
import type {
  LivestockLabourRegime,
  LivestockQuotaParams,
  LivestockQuotaPolicy,
  LivestockUGGFactor,
} from "../../api/types";

const LABOUR_REGIMES: LivestockLabourRegime[] = ["auto", "familiar", "formal"];

/** The five commercial categories the UGG matrix prices, in column order. */
const FACTOR_CATEGORIES = ["calves", "growing", "breeding", "fattening", "cull"] as const;
type FactorCategory = (typeof FACTOR_CATEGORIES)[number];

const paramsOf = (policy: LivestockQuotaPolicy): LivestockQuotaParams => ({
  priceCalfPerKg: policy.priceCalfPerKg,
  priceGrowingPerKg: policy.priceGrowingPerKg,
  priceFatteningPerKg: policy.priceFatteningPerKg,
  weightCalfKg: policy.weightCalfKg,
  weightGrowingKg: policy.weightGrowingKg,
  weightFatteningKg: policy.weightFatteningKg,
  milkingShare: policy.milkingShare,
  pricePerLitre: policy.pricePerLitre,
  vaccineCostPerUggYear: policy.vaccineCostPerUggYear,
  saltCostPerUggYear: policy.saltCostPerUggYear,
  supplementCostPerUggYear: policy.supplementCostPerUggYear,
  vetCostPerUggYear: policy.vetCostPerUggYear,
  dailyWage: policy.dailyWage,
  labourRegime: policy.labourRegime,
  uggThresholdFamily: policy.uggThresholdFamily,
  uggThresholdOccasional: policy.uggThresholdOccasional,
  uggThresholdPartial: policy.uggThresholdPartial,
  occasionalDaysPerMonth: policy.occasionalDaysPerMonth,
  fenceCostPerHaYear: policy.fenceCostPerHaYear,
  infraCostPerUggYear: policy.infraCostPerUggYear,
  hectaresPerUgg: policy.hectaresPerUgg,
  ltvPercent: policy.ltvPercent,
  annualRatePct: policy.annualRatePct,
  capitalCostPct: policy.capitalCostPct,
  maxQuota: policy.maxQuota,
  termMonths: policy.termMonths,
  applyInputCeiling: policy.applyInputCeiling,
  inputHeadroom: policy.inputHeadroom,
  factors: policy.factors.map((factor) => ({ ...factor })),
});

const toNumber = (raw: string): number => {
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? parsed : NaN;
};

const AdminLivestockQuota: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { showSuccess, showError } = useNotification();

  const [tab, setTab] = useState(0);
  const [description, setDescription] = useState("");
  const [params, setParams] = useState<LivestockQuotaParams | null>(null);
  const [activateOnSave, setActivateOnSave] = useState(true);

  const { data: active, isLoading } = useQuery({
    queryKey: ["activeLivestockQuotaPolicy"],
    queryFn: adminGetActiveLivestockQuotaPolicy,
  });

  const { data: versions } = useQuery({
    queryKey: ["livestockQuotaPolicies"],
    queryFn: () => adminGetLivestockQuotaPolicies(1, 50),
  });

  // Load the active policy into the editor, and reload it whenever another
  // version becomes active, so the editor always starts from what is in service.
  useEffect(() => {
    if (!active) return;
    setDescription("");
    setParams(paramsOf(active));
  }, [active?.id]);

  const dirty = useMemo(() => {
    if (!active || !params) return false;
    return JSON.stringify(params) !== JSON.stringify(paramsOf(active));
  }, [active, params]);

  // The same invariants the domain enforces, checked here so an operator sees
  // the problem before the request is rejected.
  const problems = useMemo(() => {
    if (!params) return [] as string[];
    const found: string[] = [];
    const family = toNumber(params.uggThresholdFamily);
    const occasional = toNumber(params.uggThresholdOccasional);
    const partial = toNumber(params.uggThresholdPartial);
    if (!(occasional > family) || !(partial > occasional)) {
      found.push(t("livestock.policy.error.thresholdOrder"));
    }
    if (!(toNumber(params.maxQuota) > 0)) {
      found.push(t("livestock.policy.error.maxQuota"));
    }
    if (!(params.termMonths >= 1)) {
      found.push(t("livestock.policy.error.termMonths"));
    }
    const share = toNumber(params.milkingShare);
    if (!(share >= 0 && share <= 1)) {
      found.push(t("livestock.policy.error.milkingShare"));
    }
    const negatives = [
      params.ltvPercent,
      params.annualRatePct,
      params.capitalCostPct,
      params.inputHeadroom,
      params.priceCalfPerKg,
      params.priceGrowingPerKg,
      params.priceFatteningPerKg,
      params.dailyWage,
    ];
    if (negatives.some((raw) => toNumber(raw) < 0)) {
      found.push(t("livestock.policy.error.negative"));
    }
    if (
      params.factors.some((factor) =>
        FACTOR_CATEGORIES.some((category) => toNumber(factor[category]) < 0)
      )
    ) {
      found.push(t("livestock.policy.error.negativeFactor"));
    }
    return found;
  }, [params, t]);

  const saveVersion = useMutation({
    mutationFn: () => {
      if (!active || !params) throw new Error("no policy loaded");
      return adminSaveLivestockQuotaPolicyVersion(active.id, {
        description,
        params,
        activate: activateOnSave,
      });
    },
    onSuccess: (policy) => {
      showSuccess(t("livestock.policy.saved", { version: policy.version }));
      queryClient.invalidateQueries({ queryKey: ["activeLivestockQuotaPolicy"] });
      queryClient.invalidateQueries({ queryKey: ["livestockQuotaPolicies"] });
    },
    onError: (error) => showError(getErrorMessage(error, t("livestock.policy.saveFailed"))),
  });

  const activateVersion = useMutation({
    mutationFn: (id: string) => adminActivateLivestockQuotaPolicy(id),
    onSuccess: (policy) => {
      showSuccess(t("livestock.policy.activated", { version: policy.version }));
      queryClient.invalidateQueries({ queryKey: ["activeLivestockQuotaPolicy"] });
      queryClient.invalidateQueries({ queryKey: ["livestockQuotaPolicies"] });
    },
    onError: (error) => showError(getErrorMessage(error, t("livestock.policy.activateFailed"))),
  });

  const updateParam = <K extends keyof LivestockQuotaParams>(
    key: K,
    value: LivestockQuotaParams[K]
  ) => setParams((prev) => (prev ? { ...prev, [key]: value } : prev));

  const updateFactor = (index: number, category: FactorCategory, value: string) =>
    setParams((prev) =>
      prev
        ? {
            ...prev,
            factors: prev.factors.map((factor, i) =>
              i === index ? { ...factor, [category]: value } : factor
            ),
          }
        : prev
    );

  const reset = () => {
    if (!active) return;
    setDescription("");
    setParams(paramsOf(active));
  };

  if (isLoading || !active || !params) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  const decimalField = (
    key: keyof LivestockQuotaParams,
    label: string,
    helper?: string
  ) => (
    <TextField
      fullWidth
      label={label}
      value={params[key] as string}
      onChange={(e) => updateParam(key, e.target.value as never)}
      helperText={helper ?? " "}
    />
  );

  const intField = (key: keyof LivestockQuotaParams, label: string, helper?: string) => (
    <TextField
      fullWidth
      type="number"
      label={label}
      value={params[key] as number}
      onChange={(e) => updateParam(key, Number(e.target.value) as never)}
      helperText={helper ?? " "}
    />
  );

  const factorRows: LivestockUGGFactor[] = params.factors;

  return (
    <Box>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", mb: 1 }}>
        <Box>
          <Typography variant="h4" gutterBottom>
            {t("livestock.policy.title")}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {t("livestock.policy.subtitle")}
          </Typography>
        </Box>
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip
            label={t("livestock.policy.activeVersion", {
              name: active.name,
              version: active.version,
            })}
            color="success"
            variant="outlined"
          />
        </Stack>
      </Box>

      {problems.length > 0 && (
        <Alert severity="error" sx={{ mb: 2 }}>
          <Stack spacing={0.5}>
            {problems.map((problem) => (
              <span key={problem}>{problem}</span>
            ))}
          </Stack>
        </Alert>
      )}

      <Card>
        <Tabs
          value={tab}
          onChange={(_, v) => setTab(v)}
          variant="scrollable"
          scrollButtons="auto"
          sx={{ px: 2, borderBottom: 1, borderColor: "divider" }}
        >
          <Tab label={t("livestock.policy.incomeTab")} />
          <Tab label={t("livestock.policy.costsTab")} />
          <Tab label={t("livestock.policy.creditTab")} />
          <Tab label={t("livestock.policy.matrixTab")} />
          <Tab label={t("livestock.policy.versionsTab")} />
        </Tabs>

        <CardContent>
          {tab === 0 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("livestock.policy.incomeHelp")}
              </Typography>
              <Grid container spacing={2}>
                <Grid item xs={12}>
                  <Typography variant="subtitle2" color="text.secondary">
                    {t("livestock.policy.pricesSection")}
                  </Typography>
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField("priceCalfPerKg", t("livestock.policy.priceCalfPerKg"))}
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField("priceGrowingPerKg", t("livestock.policy.priceGrowingPerKg"))}
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField("priceFatteningPerKg", t("livestock.policy.priceFatteningPerKg"))}
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField("weightCalfKg", t("livestock.policy.weightCalfKg"))}
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField("weightGrowingKg", t("livestock.policy.weightGrowingKg"))}
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField("weightFatteningKg", t("livestock.policy.weightFatteningKg"))}
                </Grid>

                <Grid item xs={12}>
                  <Divider sx={{ my: 1 }} />
                  <Typography variant="subtitle2" color="text.secondary">
                    {t("livestock.policy.milkSection")}
                  </Typography>
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField(
                    "milkingShare",
                    t("livestock.policy.milkingShare"),
                    t("livestock.policy.fractionHint")
                  )}
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  {decimalField("pricePerLitre", t("livestock.policy.pricePerLitre"))}
                </Grid>
              </Grid>
            </Box>
          )}

          {tab === 1 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("livestock.policy.costsHelp")}
              </Typography>
              <Grid container spacing={2}>
                <Grid item xs={12}>
                  <Typography variant="subtitle2" color="text.secondary">
                    {t("livestock.policy.basketSection")}
                  </Typography>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("vaccineCostPerUggYear", t("livestock.policy.vaccineCost"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("saltCostPerUggYear", t("livestock.policy.saltCost"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("supplementCostPerUggYear", t("livestock.policy.supplementCost"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("vetCostPerUggYear", t("livestock.policy.vetCost"))}
                </Grid>

                <Grid item xs={12}>
                  <Divider sx={{ my: 1 }} />
                  <Typography variant="subtitle2" color="text.secondary">
                    {t("livestock.policy.labourSection")}
                  </Typography>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  <TextField
                    fullWidth
                    select
                    label={t("livestock.policy.labourRegime")}
                    value={params.labourRegime}
                    onChange={(e) =>
                      updateParam("labourRegime", e.target.value as LivestockLabourRegime)
                    }
                    helperText={t("livestock.policy.labourRegimeHint")}
                  >
                    {LABOUR_REGIMES.map((regime) => (
                      <MenuItem key={regime} value={regime}>
                        {t(`livestock.policy.regime.${regime}`)}
                      </MenuItem>
                    ))}
                  </TextField>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("dailyWage", t("livestock.policy.dailyWage"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {intField("occasionalDaysPerMonth", t("livestock.policy.occasionalDays"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField(
                    "uggThresholdFamily",
                    t("livestock.policy.uggThresholdFamily"),
                    t("livestock.policy.thresholdHint")
                  )}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("uggThresholdOccasional", t("livestock.policy.uggThresholdOccasional"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("uggThresholdPartial", t("livestock.policy.uggThresholdPartial"))}
                </Grid>

                <Grid item xs={12}>
                  <Divider sx={{ my: 1 }} />
                  <Typography variant="subtitle2" color="text.secondary">
                    {t("livestock.policy.upkeepSection")}
                  </Typography>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("fenceCostPerHaYear", t("livestock.policy.fenceCost"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("infraCostPerUggYear", t("livestock.policy.infraCost"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("hectaresPerUgg", t("livestock.policy.hectaresPerUgg"))}
                </Grid>
              </Grid>
              <Alert severity="info" sx={{ mt: 2 }}>
                {t("livestock.policy.basketNote")}
              </Alert>
            </Box>
          )}

          {tab === 2 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("livestock.policy.creditHelp")}
              </Typography>
              <Grid container spacing={2}>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField(
                    "annualRatePct",
                    t("livestock.policy.annualRatePct"),
                    t("livestock.policy.percentHint")
                  )}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField(
                    "capitalCostPct",
                    t("livestock.policy.capitalCostPct"),
                    t("livestock.policy.percentHint")
                  )}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField(
                    "ltvPercent",
                    t("livestock.policy.ltvPercent"),
                    t("livestock.policy.percentHint")
                  )}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {intField("termMonths", t("livestock.policy.termMonths"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField("maxQuota", t("livestock.policy.maxQuota"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {decimalField(
                    "inputHeadroom",
                    t("livestock.policy.inputHeadroom"),
                    t("livestock.policy.headroomHint")
                  )}
                </Grid>
                <Grid item xs={12} sm={6} md={6}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={params.applyInputCeiling}
                        onChange={(e) => updateParam("applyInputCeiling", e.target.checked)}
                      />
                    }
                    label={t("livestock.policy.applyInputCeiling")}
                  />
                </Grid>
              </Grid>
              <Alert severity={params.applyInputCeiling ? "info" : "warning"} sx={{ mt: 2 }}>
                {params.applyInputCeiling
                  ? t("livestock.policy.inputCeilingOn")
                  : t("livestock.policy.inputCeilingOff")}
              </Alert>
            </Box>
          )}

          {tab === 3 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("livestock.policy.matrixHelp")}
              </Typography>
              <Paper variant="outlined" sx={{ overflowX: "auto" }}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>{t("livestock.policy.typology")}</TableCell>
                      {FACTOR_CATEGORIES.map((category) => (
                        <TableCell key={category}>
                          {t(`livestock.policy.category.${category}`)}
                        </TableCell>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {factorRows.map((factor, index) => (
                      <TableRow key={factor.typology}>
                        <TableCell>
                          <Chip label={factor.typology} size="small" />
                        </TableCell>
                        {FACTOR_CATEGORIES.map((category) => (
                          <TableCell key={category}>
                            <TextField
                              size="small"
                              value={factor[category]}
                              onChange={(e) => updateFactor(index, category, e.target.value)}
                              sx={{ width: 110 }}
                            />
                          </TableCell>
                        ))}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Paper>
              <Alert severity="info" sx={{ mt: 2 }}>
                {t("livestock.policy.matrixNote")}
              </Alert>
            </Box>
          )}

          {tab === 4 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("livestock.policy.versionsHelp")}
              </Typography>
              <Paper variant="outlined" sx={{ overflowX: "auto" }}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>{t("livestock.policy.version")}</TableCell>
                      <TableCell>{t("livestock.policy.descriptionLabel")}</TableCell>
                      <TableCell>{t("livestock.policy.updatedAt")}</TableCell>
                      <TableCell align="right">{t("common.actions")}</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {(versions?.data ?? []).map((policy) => (
                      <TableRow key={policy.id} selected={policy.isActive}>
                        <TableCell>
                          {policy.name} v{policy.version}
                          {policy.isActive && (
                            <Chip
                              size="small"
                              color="success"
                              label={t("livestock.policy.active")}
                              sx={{ ml: 1 }}
                            />
                          )}
                        </TableCell>
                        <TableCell sx={{ maxWidth: 320 }}>
                          <Typography variant="body2" noWrap>
                            {policy.description}
                          </Typography>
                        </TableCell>
                        <TableCell>{new Date(policy.updatedAt).toLocaleString()}</TableCell>
                        <TableCell align="right">
                          <Button
                            size="small"
                            startIcon={<ActivateIcon />}
                            disabled={policy.isActive || activateVersion.isPending}
                            onClick={() => activateVersion.mutate(policy.id)}
                          >
                            {t("livestock.policy.activate")}
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Paper>
            </Box>
          )}
        </CardContent>

        <Divider />
        <Box sx={{ p: 2 }}>
          <TextField
            fullWidth
            label={t("livestock.policy.descriptionLabel")}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            helperText={t("livestock.policy.descriptionHelp")}
          />
        </Box>
        <Divider />
        <Box
          sx={{
            p: 2,
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            flexWrap: "wrap",
            gap: 1,
          }}
        >
          <FormControlLabel
            control={
              <Switch
                checked={activateOnSave}
                onChange={(e) => setActivateOnSave(e.target.checked)}
              />
            }
            label={t("livestock.policy.activateOnSave")}
          />
          <Stack direction="row" spacing={1}>
            <Button startIcon={<ResetIcon />} disabled={!dirty} onClick={reset}>
              {t("livestock.policy.discard")}
            </Button>
            <Button
              variant="contained"
              startIcon={<SaveIcon />}
              disabled={!dirty || saveVersion.isPending || problems.length > 0}
              onClick={() => saveVersion.mutate()}
            >
              {t("livestock.policy.saveVersion")}
            </Button>
          </Stack>
        </Box>
      </Card>
    </Box>
  );
};

export default AdminLivestockQuota;
