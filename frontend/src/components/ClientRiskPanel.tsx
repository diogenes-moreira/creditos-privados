import React, { useState } from "react";
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
  Grid,
  Stack,
  Typography,
} from "@mui/material";
import {
  Assessment as EvaluateIcon,
  PlaylistAddCheck as ApplyIcon,
  Edit as EditIcon,
} from "@mui/icons-material";
import {
  adminApplyRiskRecommendation,
  adminEvaluateClientRisk,
  adminGetClientCreditAvailability,
  adminGetClientRiskAssessments,
  adminGetClientRiskProfile,
  adminSubmitRiskProfile,
} from "../api/endpoints";
import { getErrorMessage } from "../api/errorUtils";
import { useNotification } from "../contexts/NotificationContext";
import RiskAssessmentCard from "./RiskAssessmentCard";
import RiskDataForm from "./RiskDataForm";
import MoneyDisplay from "./MoneyDisplay";
import type { SubmitRiskProfileRequest } from "../api/types";

interface ClientRiskPanelProps {
  clientId: string;
}

const ClientRiskPanel: React.FC<ClientRiskPanelProps> = ({ clientId }) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { showSuccess, showError } = useNotification();
  const [editing, setEditing] = useState(false);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["clientRiskProfile", clientId] });
    queryClient.invalidateQueries({ queryKey: ["clientRiskAssessments", clientId] });
    queryClient.invalidateQueries({ queryKey: ["clientCreditAvailability", clientId] });
    queryClient.invalidateQueries({ queryKey: ["clientCreditLines", clientId] });
  };

  const { data: profile, isLoading: profileLoading } = useQuery({
    queryKey: ["clientRiskProfile", clientId],
    queryFn: () => adminGetClientRiskProfile(clientId),
    retry: false,
  });

  const { data: assessments } = useQuery({
    queryKey: ["clientRiskAssessments", clientId],
    queryFn: () => adminGetClientRiskAssessments(clientId, 1, 10),
    retry: false,
  });

  const { data: availability } = useQuery({
    queryKey: ["clientCreditAvailability", clientId],
    queryFn: () => adminGetClientCreditAvailability(clientId),
    retry: false,
  });

  const saveProfile = useMutation({
    mutationFn: (data: SubmitRiskProfileRequest) =>
      adminSubmitRiskProfile({ ...data, clientId }),
    onSuccess: () => {
      showSuccess(t("risk.admin.dataSaved"));
      setEditing(false);
      invalidate();
    },
    onError: (error) => showError(getErrorMessage(error, t("risk.admin.saveFailed"))),
  });

  const evaluate = useMutation({
    mutationFn: () => adminEvaluateClientRisk(clientId),
    onSuccess: () => {
      showSuccess(t("risk.admin.evaluated"));
      invalidate();
    },
    onError: (error) => showError(getErrorMessage(error, t("risk.admin.evaluateFailed"))),
  });

  const apply = useMutation({
    mutationFn: () => adminApplyRiskRecommendation(clientId),
    onSuccess: () => {
      showSuccess(t("risk.admin.applied"));
      invalidate();
    },
    onError: (error) => showError(getErrorMessage(error, t("risk.admin.applyFailed"))),
  });

  if (profileLoading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  const latest = assessments?.data?.[0];
  const canApply = latest?.decision === "approve";

  return (
    <Box>
      {!profile && !editing && (
        <Alert
          severity="info"
          sx={{ mb: 2 }}
          action={
            <Button size="small" onClick={() => setEditing(true)}>
              {t("risk.admin.addData")}
            </Button>
          }
        >
          {t("risk.admin.noData")}
        </Alert>
      )}

      {profile && !editing && (
        <Card variant="outlined" sx={{ mb: 2 }}>
          <CardContent>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
              <Typography variant="subtitle1" fontWeight={600}>
                {t("risk.admin.dataTitle")}
              </Typography>
              <Button size="small" startIcon={<EditIcon />} onClick={() => setEditing(true)}>
                {t("common.edit")}
              </Button>
            </Box>
            <Grid container spacing={2}>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.form.monthlyIncome")}
                </Typography>
                <Typography variant="body1">
                  <MoneyDisplay amount={profile.grossMonthlyIncome} />
                </Typography>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.form.monthlyExpenses")}
                </Typography>
                <Typography variant="body1">
                  <MoneyDisplay amount={profile.monthlyExpenses} />
                </Typography>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.debtToIncome")}
                </Typography>
                <Typography variant="body1">
                  {(Number(profile.debtToIncomeRatio) * 100).toFixed(1)}%
                </Typography>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.form.employmentType")}
                </Typography>
                <Typography variant="body1">
                  {t(`risk.employment.${profile.employmentType}`)}
                </Typography>
              </Grid>
            </Grid>
            <Stack direction="row" spacing={1} sx={{ mt: 2 }} flexWrap="wrap" useFlexGap>
              <Chip
                size="small"
                label={
                  profile.incomeVerified
                    ? t("risk.admin.incomeVerified")
                    : t("risk.admin.incomeUnverified")
                }
                color={profile.incomeVerified ? "success" : "default"}
                variant="outlined"
              />
              <Chip
                size="small"
                variant="outlined"
                label={t("risk.admin.dependents", { count: profile.dependents })}
              />
              <Chip size="small" variant="outlined" label={t(`risk.housing.${profile.housingStatus}`)} />
              {profile.bureauScore != null && (
                <Chip
                  size="small"
                  variant="outlined"
                  label={`${t("risk.form.bureauScore")}: ${profile.bureauScore}/${profile.bureauScoreScale}`}
                />
              )}
              <Chip
                size="small"
                variant="outlined"
                label={t("risk.admin.source", { source: profile.source })}
              />
            </Stack>
          </CardContent>
        </Card>
      )}

      {editing && (
        <Card variant="outlined" sx={{ mb: 2 }}>
          <CardContent>
            <Typography variant="subtitle1" fontWeight={600} gutterBottom>
              {t("risk.admin.dataTitle")}
            </Typography>
            <RiskDataForm
              profile={profile}
              showUnderwriterFields
              onSubmit={(data) => saveProfile.mutate(data)}
              isSubmitting={saveProfile.isPending}
            />
            <Box sx={{ mt: 1, display: "flex", justifyContent: "flex-end" }}>
              <Button size="small" onClick={() => setEditing(false)}>
                {t("common.cancel")}
              </Button>
            </Box>
          </CardContent>
        </Card>
      )}

      <Stack direction="row" spacing={1} sx={{ mb: 2 }} flexWrap="wrap" useFlexGap>
        <Button
          variant="contained"
          startIcon={<EvaluateIcon />}
          disabled={!profile || evaluate.isPending}
          onClick={() => evaluate.mutate()}
        >
          {evaluate.isPending ? t("risk.admin.evaluating") : t("risk.admin.evaluate")}
        </Button>
        <Button
          variant="outlined"
          startIcon={<ApplyIcon />}
          disabled={!canApply || apply.isPending}
          onClick={() => apply.mutate()}
        >
          {t("risk.admin.applyRecommendation")}
        </Button>
      </Stack>

      {latest && !canApply && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t("risk.admin.applyBlocked", { decision: t(`risk.decision.${latest.decision}`) })}
        </Alert>
      )}

      {availability && (
        <Card variant="outlined" sx={{ mb: 2 }}>
          <CardContent>
            <Typography variant="subtitle1" fontWeight={600} gutterBottom>
              {t("risk.admin.availabilityTitle")}
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.recommendedLimit")}
                </Typography>
                <Typography variant="h6">
                  <MoneyDisplay amount={availability.recommendedLimit} variant="h6" />
                </Typography>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.grantedLimit")}
                </Typography>
                <Typography variant="h6">
                  <MoneyDisplay amount={availability.grantedLimit} variant="h6" />
                </Typography>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.usedAmount")}
                </Typography>
                <Typography variant="h6">
                  <MoneyDisplay amount={availability.usedAmount} variant="h6" />
                </Typography>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.availableAmount")}
                </Typography>
                <Typography variant="h6" color="primary">
                  <MoneyDisplay amount={availability.availableAmount} variant="h6" fontWeight={600} />
                </Typography>
              </Grid>
            </Grid>
            {availability.stale && (
              <Alert severity="warning" sx={{ mt: 2 }}>
                {t("risk.admin.staleNotice")}
              </Alert>
            )}
          </CardContent>
        </Card>
      )}

      {latest ? (
        <RiskAssessmentCard assessment={latest} title={t("risk.admin.latestAssessment")} />
      ) : (
        profile && (
          <Alert severity="info">{t("risk.admin.notEvaluatedYet")}</Alert>
        )
      )}

      {assessments && assessments.data.length > 1 && (
        <>
          <Divider sx={{ my: 3 }} />
          <Typography variant="subtitle2" gutterBottom>
            {t("risk.admin.history")}
          </Typography>
          <Stack spacing={2}>
            {assessments.data.slice(1).map((assessment) => (
              <RiskAssessmentCard key={assessment.id} assessment={assessment} />
            ))}
          </Stack>
        </>
      )}
    </Box>
  );
};

export default ClientRiskPanel;
