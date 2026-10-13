import React, { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Box,
  TextField,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControlLabel,
  Checkbox,
  Grid,
} from "@mui/material";
import { adminRegisterClient } from "../api/endpoints";
import { useNotification } from "../contexts/NotificationContext";
import { getErrorMessage } from "../api/errorUtils";
import type { Client, RegisterRequest } from "../api/types";
import LocationSelector from "./LocationSelector";
import { useCountryConfig, countryNameFor } from "../config/countryConfig";

const emptyForm: RegisterRequest = {
  email: "", firstName: "", lastName: "",
  dni: "", cuit: "", dateOfBirth: "", phone: "",
  address: "", country: "Argentina", city: "", province: "", isPEP: false,
};

interface RegisterClientDialogProps {
  open: boolean;
  onClose: () => void;
  /** Values to start from, e.g. what the vaccination registry holds. */
  initial?: Partial<RegisterRequest>;
  /** Fields that were inferred rather than read, shown with a review hint. */
  toReview?: (keyof RegisterRequest)[];
  /** Shown above the form, to say where the pre-filled data came from. */
  notice?: React.ReactNode;
  onCreated?: (client: Client) => void;
}

/** The administrator's client registration form. */
const RegisterClientDialog: React.FC<RegisterClientDialogProps> = ({
  open,
  onClose,
  initial,
  toReview = [],
  notice,
  onCreated,
}) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const config = useCountryConfig();
  const { showSuccess, showError } = useNotification();

  const blank = (): RegisterRequest => ({ ...emptyForm, country: countryNameFor(config.country), ...initial });
  const [formData, setFormData] = useState<RegisterRequest>(blank);

  // Start from the given values each time the dialog opens, so a second
  // lookup does not inherit the first rancher's data. Only on opening: a
  // parent re-rendering must not wipe what the operator has typed.
  useEffect(() => {
    if (open) setFormData(blank());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const registerMutation = useMutation({
    mutationFn: adminRegisterClient,
    onSuccess: (client) => {
      queryClient.invalidateQueries({ queryKey: ["admin-clients"] });
      queryClient.invalidateQueries({ queryKey: ["admin-clients-search"] });
      showSuccess(t("admin.clientCreated"));
      onClose();
      onCreated?.(client);
    },
    onError: (err: unknown) => showError(getErrorMessage(err, t("admin.clientCreateError"))),
  });

  const updateField = (field: keyof RegisterRequest) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setFormData({ ...formData, [field]: e.target.value });

  // An inferred field keeps its hint until someone edits it.
  const review = (field: keyof RegisterRequest) =>
    toReview.includes(field) && formData[field] === initial?.[field]
      ? {
          helperText: t("registration.reviewInferred"),
          FormHelperTextProps: { sx: { color: "warning.main" } },
        }
      : {};

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t("admin.registerClient")}</DialogTitle>
      <DialogContent>
        <Box display="flex" flexDirection="column" gap={2} mt={1}>
          {notice}
          <TextField label={t("auth.email")} type="email" value={formData.email} onChange={updateField("email")} />
          <Box display="flex" gap={2}>
            <TextField label={t("registration.firstName")} value={formData.firstName} onChange={updateField("firstName")} fullWidth {...review("firstName")} />
            <TextField label={t("registration.lastName")} value={formData.lastName} onChange={updateField("lastName")} fullWidth {...review("lastName")} />
          </Box>
          <Box display="flex" gap={2}>
            <TextField label={config.nationalIdLabel} value={formData.dni} onChange={updateField("dni")} fullWidth />
            <TextField label={config.taxIdLabel} value={formData.cuit} onChange={updateField("cuit")} fullWidth {...review("cuit")} />
          </Box>
          <TextField label={t("registration.dateOfBirth")} type="date" value={formData.dateOfBirth} onChange={updateField("dateOfBirth")} InputLabelProps={{ shrink: true }} />
          <TextField label={t("registration.phone")} value={formData.phone} onChange={updateField("phone")} />
          <TextField label={t("registration.address")} value={formData.address} onChange={updateField("address")} {...review("address")} />
          <Grid container spacing={2}>
            <LocationSelector
              country={formData.country}
              province={formData.province}
              city={formData.city}
              onChange={(field, value) => setFormData(prev => ({ ...prev, [field]: value }))}
            />
          </Grid>
          <FormControlLabel
            control={<Checkbox checked={formData.isPEP} onChange={(e) => setFormData({ ...formData, isPEP: e.target.checked })} />}
            label={t("registration.isPEP")}
          />
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={onClose}>{t("common.cancel")}</Button>
        <Button
          variant="contained"
          onClick={() => registerMutation.mutate(formData)}
          disabled={
            registerMutation.isPending ||
            !formData.email || !formData.firstName || !formData.lastName ||
            !formData.dni || !formData.cuit || !formData.dateOfBirth || !formData.phone
          }
        >
          {registerMutation.isPending ? t("common.creating") : t("common.create")}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default RegisterClientDialog;
