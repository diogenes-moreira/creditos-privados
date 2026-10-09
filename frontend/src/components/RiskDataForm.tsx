import React from "react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import {
  Box,
  Grid,
  TextField,
  Button,
  MenuItem,
  Switch,
  FormControlLabel,
  Typography,
  Divider,
  InputAdornment,
} from "@mui/material";
import type {
  EmploymentType,
  HousingStatus,
  RiskProfile,
  SubmitRiskProfileRequest,
} from "../api/types";
import { getCountryConfig } from "../config/countryConfig";

const EMPLOYMENT_TYPES: EmploymentType[] = [
  "employee",
  "self_employed",
  "business_owner",
  "retired",
  "informal",
  "unemployed",
];

const HOUSING_STATUSES: HousingStatus[] = ["owned", "mortgaged", "rented", "family", "other"];

// Amounts stay strings end to end so decimal precision survives the round trip
// to the API, which also speaks decimal strings.
const amount = (required: boolean) =>
  z
    .string()
    .transform((v) => v.trim())
    .refine((v) => (required ? v !== "" : true), { message: "required" })
    .refine((v) => v === "" || (!Number.isNaN(Number(v)) && Number(v) >= 0), {
      message: "invalid",
    });

const riskDataSchema = z.object({
  monthlyIncome: amount(true),
  additionalIncome: amount(false),
  incomeVerified: z.boolean(),
  employmentType: z.enum([
    "employee",
    "self_employed",
    "business_owner",
    "retired",
    "informal",
    "unemployed",
  ]),
  employerName: z.string(),
  monthsEmployed: z.coerce.number().int().min(0).max(720),
  monthlyExpenses: amount(false),
  existingDebtPayments: amount(false),
  dependents: z.coerce.number().int().min(0).max(20),
  housingStatus: z.enum(["owned", "mortgaged", "rented", "family", "other"]),
  hasBankAccount: z.boolean(),
  bureauScore: z.string(),
  adverseRecords: z.coerce.number().int().min(0).max(50),
  notes: z.string(),
});

type RiskDataFormValues = z.infer<typeof riskDataSchema>;

const emptyValues: RiskDataFormValues = {
  monthlyIncome: "",
  additionalIncome: "",
  incomeVerified: false,
  employmentType: "employee",
  employerName: "",
  monthsEmployed: 0,
  monthlyExpenses: "",
  existingDebtPayments: "",
  dependents: 0,
  housingStatus: "rented",
  hasBankAccount: false,
  bureauScore: "",
  adverseRecords: 0,
  notes: "",
};

const toFormValues = (profile?: RiskProfile): RiskDataFormValues =>
  profile
    ? {
        monthlyIncome: profile.monthlyIncome,
        additionalIncome: profile.additionalIncome,
        incomeVerified: profile.incomeVerified,
        employmentType: profile.employmentType,
        employerName: profile.employerName ?? "",
        monthsEmployed: profile.monthsEmployed,
        monthlyExpenses: profile.monthlyExpenses,
        existingDebtPayments: profile.existingDebtPayments,
        dependents: profile.dependents,
        housingStatus: profile.housingStatus,
        hasBankAccount: profile.hasBankAccount,
        bureauScore: profile.bureauScore != null ? String(profile.bureauScore) : "",
        adverseRecords: profile.adverseRecords,
        notes: profile.notes ?? "",
      }
    : emptyValues;

interface RiskDataFormProps {
  profile?: RiskProfile;
  onSubmit: (data: SubmitRiskProfileRequest) => void;
  isSubmitting?: boolean;
  /** Bureau data and income verification are operator judgements, not
   *  self-reported facts, so they are hidden from the client-facing form. */
  showUnderwriterFields?: boolean;
  submitLabel?: string;
}

const RiskDataForm: React.FC<RiskDataFormProps> = ({
  profile,
  onSubmit,
  isSubmitting = false,
  showUnderwriterFields = false,
  submitLabel,
}) => {
  const { t } = useTranslation();
  const { currency } = getCountryConfig();

  const {
    control,
    handleSubmit,
    formState: { errors },
  } = useForm<RiskDataFormValues>({
    resolver: zodResolver(riskDataSchema),
    defaultValues: toFormValues(profile),
    values: profile ? toFormValues(profile) : undefined,
  });

  const submit = (values: RiskDataFormValues) => {
    const parsedScore = values.bureauScore.trim() === "" ? null : Number(values.bureauScore);
    onSubmit({
      monthlyIncome: values.monthlyIncome,
      additionalIncome: values.additionalIncome || "0",
      incomeVerified: values.incomeVerified,
      employmentType: values.employmentType,
      employerName: values.employerName,
      monthsEmployed: values.monthsEmployed,
      monthlyExpenses: values.monthlyExpenses || "0",
      existingDebtPayments: values.existingDebtPayments || "0",
      dependents: values.dependents,
      housingStatus: values.housingStatus,
      hasBankAccount: values.hasBankAccount,
      bureauScore: parsedScore,
      adverseRecords: values.adverseRecords,
      notes: values.notes,
    });
  };

  const money = (name: keyof RiskDataFormValues, label: string, required = false) => (
    <Controller
      name={name}
      control={control}
      render={({ field }) => (
        <TextField
          {...field}
          fullWidth
          label={label}
          required={required}
          error={!!errors[name]}
          helperText={errors[name] ? t("risk.form.invalidAmount") : " "}
          InputProps={{
            startAdornment: <InputAdornment position="start">{currency}</InputAdornment>,
          }}
        />
      )}
    />
  );

  const number = (name: keyof RiskDataFormValues, label: string, max: number, helper?: string) => (
    <Controller
      name={name}
      control={control}
      render={({ field }) => (
        <TextField
          {...field}
          fullWidth
          type="number"
          label={label}
          inputProps={{ min: 0, max }}
          error={!!errors[name]}
          helperText={errors[name] ? t("risk.form.invalidNumber") : helper ?? " "}
        />
      )}
    />
  );

  return (
    <Box component="form" onSubmit={handleSubmit(submit)} noValidate>
      <Typography variant="subtitle2" color="text.secondary" gutterBottom>
        {t("risk.form.incomeSection")}
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12} sm={6}>
          {money("monthlyIncome", t("risk.form.monthlyIncome"), true)}
        </Grid>
        <Grid item xs={12} sm={6}>
          {money("additionalIncome", t("risk.form.additionalIncome"))}
        </Grid>
        <Grid item xs={12} sm={6}>
          <Controller
            name="employmentType"
            control={control}
            render={({ field }) => (
              <TextField {...field} select fullWidth label={t("risk.form.employmentType")} helperText=" ">
                {EMPLOYMENT_TYPES.map((value) => (
                  <MenuItem key={value} value={value}>
                    {t(`risk.employment.${value}`)}
                  </MenuItem>
                ))}
              </TextField>
            )}
          />
        </Grid>
        <Grid item xs={12} sm={6}>
          <Controller
            name="employerName"
            control={control}
            render={({ field }) => (
              <TextField {...field} fullWidth label={t("risk.form.employerName")} helperText=" " />
            )}
          />
        </Grid>
        <Grid item xs={12} sm={6}>
          {number("monthsEmployed", t("risk.form.monthsEmployed"), 720, t("risk.form.monthsEmployedHint"))}
        </Grid>
        {showUnderwriterFields && (
          <Grid item xs={12} sm={6}>
            <Controller
              name="incomeVerified"
              control={control}
              render={({ field }) => (
                <FormControlLabel
                  control={<Switch checked={field.value} onChange={(e) => field.onChange(e.target.checked)} />}
                  label={t("risk.form.incomeVerified")}
                />
              )}
            />
          </Grid>
        )}
      </Grid>

      <Divider sx={{ my: 3 }} />
      <Typography variant="subtitle2" color="text.secondary" gutterBottom>
        {t("risk.form.obligationsSection")}
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12} sm={6}>
          {money("monthlyExpenses", t("risk.form.monthlyExpenses"))}
        </Grid>
        <Grid item xs={12} sm={6}>
          {money("existingDebtPayments", t("risk.form.existingDebtPayments"))}
        </Grid>
        <Grid item xs={12} sm={6}>
          {number("dependents", t("risk.form.dependents"), 20)}
        </Grid>
        <Grid item xs={12} sm={6}>
          <Controller
            name="housingStatus"
            control={control}
            render={({ field }) => (
              <TextField {...field} select fullWidth label={t("risk.form.housingStatus")} helperText=" ">
                {HOUSING_STATUSES.map((value) => (
                  <MenuItem key={value} value={value}>
                    {t(`risk.housing.${value}`)}
                  </MenuItem>
                ))}
              </TextField>
            )}
          />
        </Grid>
        <Grid item xs={12} sm={6}>
          <Controller
            name="hasBankAccount"
            control={control}
            render={({ field }) => (
              <FormControlLabel
                control={<Switch checked={field.value} onChange={(e) => field.onChange(e.target.checked)} />}
                label={t("risk.form.hasBankAccount")}
              />
            )}
          />
        </Grid>
      </Grid>

      {showUnderwriterFields && (
        <>
          <Divider sx={{ my: 3 }} />
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            {t("risk.form.bureauSection")}
          </Typography>
          <Grid container spacing={2}>
            <Grid item xs={12} sm={6}>
              <Controller
                name="bureauScore"
                control={control}
                render={({ field }) => (
                  <TextField
                    {...field}
                    fullWidth
                    type="number"
                    label={t("risk.form.bureauScore")}
                    inputProps={{ min: 0, max: 1000 }}
                    helperText={t("risk.form.bureauScoreHint")}
                  />
                )}
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              {number("adverseRecords", t("risk.form.adverseRecords"), 50)}
            </Grid>
          </Grid>
        </>
      )}

      <Divider sx={{ my: 3 }} />
      <Controller
        name="notes"
        control={control}
        render={({ field }) => (
          <TextField {...field} fullWidth multiline rows={3} label={t("risk.form.notes")} />
        )}
      />

      <Box sx={{ mt: 3, display: "flex", justifyContent: "flex-end" }}>
        <Button type="submit" variant="contained" disabled={isSubmitting}>
          {submitLabel ?? t("risk.form.save")}
        </Button>
      </Box>
    </Box>
  );
};

export default RiskDataForm;
