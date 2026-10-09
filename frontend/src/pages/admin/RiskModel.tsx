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
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
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
  Tooltip,
  Typography,
} from "@mui/material";
import {
  Save as SaveIcon,
  Visibility as PreviewIcon,
  CheckCircle as ActivateIcon,
  RestartAlt as ResetIcon,
} from "@mui/icons-material";
import {
  adminActivateRiskModel,
  adminGetActiveRiskModel,
  adminGetRiskAnalyzerInfo,
  adminGetRiskModels,
  adminPreviewRiskPrompt,
  adminSaveRiskModelVersion,
} from "../../api/endpoints";
import { getErrorMessage } from "../../api/errorUtils";
import { useNotification } from "../../contexts/NotificationContext";
import RiskAssessmentCard from "../../components/RiskAssessmentCard";
import type {
  PromptPreview,
  RiskDecision,
  RiskGradeBand,
  RiskModel as RiskModelType,
  RiskModelParams,
} from "../../api/types";

const DECISIONS: RiskDecision[] = ["approve", "manual_review", "decline"];

const paramsOf = (model: RiskModelType): RiskModelParams => ({
  minAge: model.minAge,
  maxAge: model.maxAge,
  minMonthlyIncome: model.minMonthlyIncome,
  maxAdverseRecords: model.maxAdverseRecords,
  maxDti: model.maxDti,
  maxPaymentToIncome: model.maxPaymentToIncome,
  unverifiedIncomeHaircut: model.unverifiedIncomeHaircut,
  expensePerDependent: model.expensePerDependent,
  absoluteMaxLimit: model.absoluteMaxLimit,
  maxInstallments: model.maxInstallments,
  limitRoundingStep: model.limitRoundingStep,
  baseInterestRate: model.baseInterestRate,
});

const AdminRiskModel: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { showSuccess, showError } = useNotification();

  const [tab, setTab] = useState(0);
  const [prompt, setPrompt] = useState("");
  const [description, setDescription] = useState("");
  const [params, setParams] = useState<RiskModelParams | null>(null);
  const [bands, setBands] = useState<RiskGradeBand[]>([]);
  const [activateOnSave, setActivateOnSave] = useState(true);
  const [preview, setPreview] = useState<PromptPreview | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);

  const { data: active, isLoading } = useQuery({
    queryKey: ["activeRiskModel"],
    queryFn: adminGetActiveRiskModel,
  });

  const { data: analyzer } = useQuery({
    queryKey: ["riskAnalyzerInfo"],
    queryFn: adminGetRiskAnalyzerInfo,
  });

  const { data: versions } = useQuery({
    queryKey: ["riskModels"],
    queryFn: () => adminGetRiskModels(1, 50),
  });

  // Load the active model into the editor once, and whenever a new version
  // becomes active, so the editor always starts from what is in service.
  useEffect(() => {
    if (!active) return;
    setPrompt(active.prompt);
    setDescription(active.description ?? "");
    setParams(paramsOf(active));
    setBands(active.bands);
  }, [active?.id]);

  const dirty = useMemo(() => {
    if (!active || !params) return false;
    return (
      prompt !== active.prompt ||
      description !== (active.description ?? "") ||
      JSON.stringify(params) !== JSON.stringify(paramsOf(active)) ||
      JSON.stringify(bands) !== JSON.stringify(active.bands)
    );
  }, [active, prompt, description, params, bands]);

  const usedPlaceholders = useMemo(() => {
    const matches = prompt.matchAll(/\{\{\s*([a-zA-Z_]+)\s*\}\}/g);
    return new Set(Array.from(matches, (m) => m[1]));
  }, [prompt]);

  const unknownPlaceholders = useMemo(() => {
    const known = new Set(active?.placeholders ?? []);
    return Array.from(usedPlaceholders).filter((p) => !known.has(p));
  }, [usedPlaceholders, active]);

  const saveVersion = useMutation({
    mutationFn: () => {
      if (!active || !params) throw new Error("no model loaded");
      return adminSaveRiskModelVersion(active.id, {
        prompt,
        description,
        params,
        bands,
        activate: activateOnSave,
      });
    },
    onSuccess: (model) => {
      showSuccess(t("risk.model.saved", { version: model.version }));
      queryClient.invalidateQueries({ queryKey: ["activeRiskModel"] });
      queryClient.invalidateQueries({ queryKey: ["riskModels"] });
    },
    onError: (error) => showError(getErrorMessage(error, t("risk.model.saveFailed"))),
  });

  const activateVersion = useMutation({
    mutationFn: (id: string) => adminActivateRiskModel(id),
    onSuccess: (model) => {
      showSuccess(t("risk.model.activated", { version: model.version }));
      queryClient.invalidateQueries({ queryKey: ["activeRiskModel"] });
      queryClient.invalidateQueries({ queryKey: ["riskModels"] });
    },
    onError: (error) => showError(getErrorMessage(error, t("risk.model.activateFailed"))),
  });

  const runPreview = useMutation({
    mutationFn: () => adminPreviewRiskPrompt(prompt),
    onSuccess: (result) => {
      setPreview(result);
      setPreviewOpen(true);
    },
    onError: (error) => showError(getErrorMessage(error, t("risk.model.previewFailed"))),
  });

  const updateParam = <K extends keyof RiskModelParams>(key: K, value: RiskModelParams[K]) =>
    setParams((prev) => (prev ? { ...prev, [key]: value } : prev));

  const updateBand = (index: number, patch: Partial<RiskGradeBand>) =>
    setBands((prev) => prev.map((band, i) => (i === index ? { ...band, ...patch } : band)));

  const reset = () => {
    if (!active) return;
    setPrompt(active.prompt);
    setDescription(active.description ?? "");
    setParams(paramsOf(active));
    setBands(active.bands);
  };

  if (isLoading || !active || !params) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  const percentField = (
    key: keyof RiskModelParams,
    label: string,
    helper?: string
  ) => (
    <TextField
      fullWidth
      label={label}
      value={params[key] as string}
      onChange={(e) => updateParam(key, e.target.value as never)}
      helperText={helper ?? t("risk.model.fractionHint")}
    />
  );

  const amountField = (key: keyof RiskModelParams, label: string) => (
    <TextField
      fullWidth
      label={label}
      value={params[key] as string}
      onChange={(e) => updateParam(key, e.target.value as never)}
      helperText=" "
    />
  );

  const intField = (key: keyof RiskModelParams, label: string) => (
    <TextField
      fullWidth
      type="number"
      label={label}
      value={params[key] as number}
      onChange={(e) => updateParam(key, Number(e.target.value) as never)}
      helperText=" "
    />
  );

  return (
    <Box>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", mb: 1 }}>
        <Box>
          <Typography variant="h4" gutterBottom>
            {t("risk.model.title")}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {t("risk.model.subtitle")}
          </Typography>
        </Box>
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip
            label={t("risk.model.activeVersion", { name: active.name, version: active.version })}
            color="success"
            variant="outlined"
          />
        </Stack>
      </Box>

      {analyzer && !analyzer.promptDriven && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t("risk.model.noAnalyzerConfigured")}
        </Alert>
      )}
      {analyzer && analyzer.promptDriven && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t("risk.model.analyzerConfigured", { analyzer: analyzer.analyzer })}
        </Alert>
      )}

      <Card>
        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ px: 2, borderBottom: 1, borderColor: "divider" }}>
          <Tab label={t("risk.model.promptTab")} />
          <Tab label={t("risk.model.guardrailsTab")} />
          <Tab label={t("risk.model.bandsTab")} />
          <Tab label={t("risk.model.versionsTab")} />
        </Tabs>

        <CardContent>
          {tab === 0 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("risk.model.promptHelp")}
              </Typography>

              <Stack direction="row" spacing={1} sx={{ mb: 2 }} flexWrap="wrap" useFlexGap>
                {active.placeholders.map((placeholder) => (
                  <Tooltip key={placeholder} title={t(`risk.model.placeholder.${placeholder}`, placeholder)}>
                    <Chip
                      label={`{{${placeholder}}}`}
                      size="small"
                      color={usedPlaceholders.has(placeholder) ? "primary" : "default"}
                      variant={usedPlaceholders.has(placeholder) ? "filled" : "outlined"}
                      onClick={() => setPrompt((prev) => `${prev}\n{{${placeholder}}}`)}
                    />
                  </Tooltip>
                ))}
              </Stack>

              {unknownPlaceholders.length > 0 && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {t("risk.model.unknownPlaceholders", { list: unknownPlaceholders.join(", ") })}
                </Alert>
              )}
              {!usedPlaceholders.has("profile") && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {t("risk.model.profileRequired")}
                </Alert>
              )}

              <TextField
                fullWidth
                multiline
                minRows={18}
                label={t("risk.model.promptLabel")}
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                sx={{ "& textarea": { fontFamily: "monospace", fontSize: 13, lineHeight: 1.6 } }}
              />

              <TextField
                fullWidth
                sx={{ mt: 2 }}
                label={t("risk.model.descriptionLabel")}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                helperText={t("risk.model.descriptionHelp")}
              />
            </Box>
          )}

          {tab === 1 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("risk.model.guardrailsHelp")}
              </Typography>
              <Grid container spacing={2}>
                <Grid item xs={12} sm={6} md={3}>
                  {intField("minAge", t("risk.model.minAge"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {intField("maxAge", t("risk.model.maxAge"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {amountField("minMonthlyIncome", t("risk.model.minMonthlyIncome"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {intField("maxAdverseRecords", t("risk.model.maxAdverseRecords"))}
                </Grid>

                <Grid item xs={12}>
                  <Divider sx={{ my: 1 }} />
                  <Typography variant="subtitle2" color="text.secondary">
                    {t("risk.model.affordabilitySection")}
                  </Typography>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {percentField("maxDti", t("risk.model.maxDti"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {percentField("maxPaymentToIncome", t("risk.model.maxPaymentToIncome"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {percentField("unverifiedIncomeHaircut", t("risk.model.unverifiedIncomeHaircut"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {amountField("expensePerDependent", t("risk.model.expensePerDependent"))}
                </Grid>

                <Grid item xs={12}>
                  <Divider sx={{ my: 1 }} />
                  <Typography variant="subtitle2" color="text.secondary">
                    {t("risk.model.exposureSection")}
                  </Typography>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {amountField("absoluteMaxLimit", t("risk.model.absoluteMaxLimit"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {intField("maxInstallments", t("risk.model.maxInstallments"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {amountField("limitRoundingStep", t("risk.model.limitRoundingStep"))}
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  {percentField("baseInterestRate", t("risk.model.baseInterestRate"), t("risk.model.rateHint"))}
                </Grid>
              </Grid>
            </Box>
          )}

          {tab === 2 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("risk.model.bandsHelp")}
              </Typography>
              <Paper variant="outlined" sx={{ overflowX: "auto" }}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>{t("risk.model.band.grade")}</TableCell>
                      <TableCell>{t("risk.model.band.minScore")}</TableCell>
                      <TableCell>{t("risk.model.band.decision")}</TableCell>
                      <TableCell>{t("risk.model.band.interestSpread")}</TableCell>
                      <TableCell>{t("risk.model.band.exposureFactor")}</TableCell>
                      <TableCell>{t("risk.model.band.maxInstallments")}</TableCell>
                      <TableCell>{t("risk.model.band.pd")}</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {bands.map((band, index) => (
                      <TableRow key={band.grade}>
                        <TableCell>
                          <Chip label={band.grade} size="small" />
                        </TableCell>
                        <TableCell>
                          <TextField
                            size="small"
                            type="number"
                            value={band.minScore}
                            inputProps={{ min: 0, max: 100 }}
                            onChange={(e) => updateBand(index, { minScore: Number(e.target.value) })}
                            sx={{ width: 90 }}
                          />
                        </TableCell>
                        <TableCell>
                          <TextField
                            size="small"
                            select
                            value={band.decision}
                            onChange={(e) => updateBand(index, { decision: e.target.value as RiskDecision })}
                            sx={{ width: 150 }}
                          >
                            {DECISIONS.map((decision) => (
                              <MenuItem key={decision} value={decision}>
                                {t(`risk.decision.${decision}`)}
                              </MenuItem>
                            ))}
                          </TextField>
                        </TableCell>
                        <TableCell>
                          <TextField
                            size="small"
                            value={band.interestSpread}
                            onChange={(e) => updateBand(index, { interestSpread: e.target.value })}
                            sx={{ width: 100 }}
                          />
                        </TableCell>
                        <TableCell>
                          <TextField
                            size="small"
                            value={band.exposureFactor}
                            onChange={(e) => updateBand(index, { exposureFactor: e.target.value })}
                            sx={{ width: 100 }}
                          />
                        </TableCell>
                        <TableCell>
                          <TextField
                            size="small"
                            type="number"
                            value={band.maxInstallments}
                            inputProps={{ min: 0, max: 60 }}
                            onChange={(e) => updateBand(index, { maxInstallments: Number(e.target.value) })}
                            sx={{ width: 90 }}
                          />
                        </TableCell>
                        <TableCell>
                          <TextField
                            size="small"
                            value={band.probabilityOfDefault}
                            onChange={(e) => updateBand(index, { probabilityOfDefault: e.target.value })}
                            sx={{ width: 100 }}
                          />
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Paper>
              <Alert severity="info" sx={{ mt: 2 }}>
                {t("risk.model.bandsNote")}
              </Alert>
            </Box>
          )}

          {tab === 3 && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                {t("risk.model.versionsHelp")}
              </Typography>
              <Paper variant="outlined" sx={{ overflowX: "auto" }}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>{t("risk.model.version")}</TableCell>
                      <TableCell>{t("risk.model.descriptionLabel")}</TableCell>
                      <TableCell>{t("risk.model.updatedAt")}</TableCell>
                      <TableCell align="right">{t("common.actions")}</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {(versions?.data ?? []).map((model) => (
                      <TableRow key={model.id} selected={model.isActive}>
                        <TableCell>
                          {model.name} v{model.version}
                          {model.isActive && (
                            <Chip size="small" color="success" label={t("risk.model.active")} sx={{ ml: 1 }} />
                          )}
                        </TableCell>
                        <TableCell sx={{ maxWidth: 320 }}>
                          <Typography variant="body2" noWrap>
                            {model.description}
                          </Typography>
                        </TableCell>
                        <TableCell>{new Date(model.updatedAt).toLocaleString()}</TableCell>
                        <TableCell align="right">
                          <Button
                            size="small"
                            startIcon={<ActivateIcon />}
                            disabled={model.isActive || activateVersion.isPending}
                            onClick={() => activateVersion.mutate(model.id)}
                          >
                            {t("risk.model.activate")}
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
        <Box sx={{ p: 2, display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 1 }}>
          <FormControlLabel
            control={
              <Switch checked={activateOnSave} onChange={(e) => setActivateOnSave(e.target.checked)} />
            }
            label={t("risk.model.activateOnSave")}
          />
          <Stack direction="row" spacing={1}>
            <Button startIcon={<ResetIcon />} disabled={!dirty} onClick={reset}>
              {t("risk.model.discard")}
            </Button>
            <Button
              startIcon={<PreviewIcon />}
              onClick={() => runPreview.mutate()}
              disabled={runPreview.isPending || !usedPlaceholders.has("profile")}
            >
              {t("risk.model.preview")}
            </Button>
            <Button
              variant="contained"
              startIcon={<SaveIcon />}
              disabled={
                !dirty ||
                saveVersion.isPending ||
                unknownPlaceholders.length > 0 ||
                !usedPlaceholders.has("profile")
              }
              onClick={() => saveVersion.mutate()}
            >
              {t("risk.model.saveVersion")}
            </Button>
          </Stack>
        </Box>
      </Card>

      <Dialog open={previewOpen} onClose={() => setPreviewOpen(false)} maxWidth="lg" fullWidth>
        <DialogTitle>{t("risk.model.previewTitle")}</DialogTitle>
        <DialogContent dividers>
          {preview && (
            <Stack spacing={2}>
              {(preview.warnings ?? []).map((warning, index) => (
                <Alert key={index} severity="warning">
                  {warning}
                </Alert>
              ))}

              <Typography variant="subtitle2">{t("risk.model.renderedPrompt")}</Typography>
              <Paper
                variant="outlined"
                sx={{
                  p: 2,
                  maxHeight: 320,
                  overflow: "auto",
                  fontFamily: "monospace",
                  fontSize: 12,
                  whiteSpace: "pre-wrap",
                }}
              >
                {preview.rendered}
              </Paper>

              <Typography variant="subtitle2">{t("risk.model.applicantPayload")}</Typography>
              <Paper
                variant="outlined"
                sx={{
                  p: 2,
                  maxHeight: 240,
                  overflow: "auto",
                  fontFamily: "monospace",
                  fontSize: 12,
                  whiteSpace: "pre-wrap",
                }}
              >
                {preview.applicant}
              </Paper>

              {preview.baseline && (
                <>
                  <Typography variant="subtitle2">{t("risk.model.baselineTitle")}</Typography>
                  <RiskAssessmentCard evaluation={preview.baseline} />
                </>
              )}
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPreviewOpen(false)}>{t("common.close")}</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default AdminRiskModel;
