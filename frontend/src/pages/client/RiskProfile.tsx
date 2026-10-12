import React from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Alert,
  Box,
  Card,
  CardContent,
  CircularProgress,
  Grid,
  Typography,
} from "@mui/material";
import { getMyCreditAvailability, getMyRiskProfile, submitMyRiskProfile } from "../../api/endpoints";
import { getErrorMessage } from "../../api/errorUtils";
import { useNotification } from "../../contexts/NotificationContext";
import RiskDataForm from "../../components/RiskDataForm";
import MoneyDisplay from "../../components/MoneyDisplay";
import type { SubmitRiskProfileRequest } from "../../api/types";

const ClientRiskProfile: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { showSuccess, showError } = useNotification();

  const { data: profile, isLoading } = useQuery({
    queryKey: ["myRiskProfile"],
    queryFn: getMyRiskProfile,
    // A client who has never submitted data gets a 404; that is an expected
    // empty state, not an error worth retrying.
    retry: false,
  });

  const { data: availability } = useQuery({
    queryKey: ["myCreditAvailability"],
    queryFn: getMyCreditAvailability,
    retry: false,
  });

  const mutation = useMutation({
    mutationFn: (data: SubmitRiskProfileRequest) => submitMyRiskProfile(data),
    onSuccess: () => {
      showSuccess(t("risk.client.saved"));
      queryClient.invalidateQueries({ queryKey: ["myRiskProfile"] });
      queryClient.invalidateQueries({ queryKey: ["myCreditAvailability"] });
    },
    onError: (error) => showError(getErrorMessage(error, t("risk.client.saveFailed"))),
  });

  if (isLoading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        {t("risk.client.title")}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        {t("risk.client.subtitle")}
      </Typography>

      {availability && (
        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="subtitle1" fontWeight={600} gutterBottom>
              {t("risk.client.availabilityTitle")}
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={12} sm={4}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.availableAmount")}
                </Typography>
                <Typography variant="h5" color="primary">
                  <MoneyDisplay amount={availability.availableAmount} variant="h5" fontWeight={600} />
                </Typography>
              </Grid>
              <Grid item xs={6} sm={4}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.recommendedLimit")}
                </Typography>
                <Typography variant="h6">
                  <MoneyDisplay amount={availability.recommendedLimit} variant="h6" />
                </Typography>
              </Grid>
              <Grid item xs={6} sm={4}>
                <Typography variant="caption" color="text.secondary">
                  {t("risk.usedAmount")}
                </Typography>
                <Typography variant="h6">
                  <MoneyDisplay amount={availability.usedAmount} variant="h6" />
                </Typography>
              </Grid>
            </Grid>
            {availability.stale && (
              <Alert severity="info" sx={{ mt: 2 }}>
                {t("risk.client.staleNotice")}
              </Alert>
            )}
          </CardContent>
        </Card>
      )}

      {!profile && (
        <Alert severity="info" sx={{ mb: 3 }}>
          {t("risk.client.noDataYet")}
        </Alert>
      )}

      <Card>
        <CardContent>
          <RiskDataForm
            profile={profile}
            onSubmit={(data) => mutation.mutate(data)}
            isSubmitting={mutation.isPending}
            submitLabel={t("risk.client.submit")}
          />
        </CardContent>
      </Card>
    </Box>
  );
};

export default ClientRiskProfile;
