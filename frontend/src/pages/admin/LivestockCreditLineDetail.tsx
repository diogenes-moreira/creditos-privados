import React, { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
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
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  ArrowBack as BackIcon,
  ContentCopy as CopyIcon,
  Download as DownloadIcon,
  OpenInNew as OpenIcon,
} from "@mui/icons-material";
import { adminDownloadPromissoryNote, adminGetLivestockCreditLine } from "../../api/endpoints";
import StatusBadge from "../../components/StatusBadge";
import LivestockCreditPortfolio from "./LivestockCreditPortfolio";
import { getErrorMessage } from "../../api/errorUtils";
import { useNotification } from "../../contexts/NotificationContext";
import type { LivestockCreditLineDetail as Detail } from "../../api/types";
import {
  formatBytes,
  formatCreditAmount,
  formatDateTime,
  shortHash,
} from "../../utils/creditLineFormat";

const httpStatusOf = (error: unknown): number | undefined =>
  (error as { response?: { status?: number } })?.response?.status;

const errorCodeOf = (error: unknown): string | undefined =>
  (error as { response?: { data?: { code?: string } } })?.response?.data?.code;

/** How long a blob URL handed to a new tab is kept before it is released. */
const BLOB_URL_LIFETIME_MS = 60_000;

const LivestockCreditLineDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { showError, showSuccess } = useNotification();
  const [busy, setBusy] = useState<"view" | "download" | null>(null);

  const { data: line, isLoading, error } = useQuery({
    queryKey: ["admin-livestock-credit-line", id],
    queryFn: () => adminGetLivestockCreditLine(id!),
    enabled: !!id,
    retry: (count, err) => httpStatusOf(err) !== 404 && count < 2,
  });

  const noteError = (err: unknown) =>
    showError(
      errorCodeOf(err) === "promissory_note_corrupted"
        ? t("livestockCreditLines.detail.noteCorrupted")
        : getErrorMessage(err, t("livestockCreditLines.detail.noteError"))
    );

  // The PDF is fetched with the session's token (a plain link would not carry
  // it) and handed to the new tab as a blob URL. The tab is opened before the
  // request so the browser treats it as the click's own pop-up.
  const viewNote = async () => {
    if (!line) return;
    const tab = window.open("", "_blank");
    if (!tab) {
      showError(t("livestockCreditLines.detail.popupBlocked"));
      return;
    }
    setBusy("view");
    try {
      const blob = await adminDownloadPromissoryNote(line.id);
      const url = URL.createObjectURL(blob);
      tab.opener = null;
      tab.location.href = url;
      window.setTimeout(() => URL.revokeObjectURL(url), BLOB_URL_LIFETIME_MS);
    } catch (err) {
      tab.close();
      noteError(err);
    } finally {
      setBusy(null);
    }
  };

  const downloadNote = async () => {
    if (!line) return;
    setBusy("download");
    try {
      const blob = await adminDownloadPromissoryNote(line.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${line.promissoryNoteNumber}.pdf`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      noteError(err);
    } finally {
      setBusy(null);
    }
  };

  const copyHash = async () => {
    if (!line) return;
    try {
      await navigator.clipboard.writeText(line.documentSha256);
      showSuccess(t("livestockCreditLines.detail.copied"));
    } catch {
      // Clipboard not available (e.g. an insecure origin): nothing to do.
    }
  };

  const back = (
    <Button startIcon={<BackIcon />} onClick={() => navigate("/admin/livestock-credit-lines")} sx={{ mb: 2 }}>
      {t("livestockCreditLines.detail.back")}
    </Button>
  );

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" py={6}>
        <CircularProgress />
      </Box>
    );
  }

  if (error || !line) {
    return (
      <Box>
        {back}
        <Alert severity={httpStatusOf(error) === 404 ? "info" : "error"}>
          {httpStatusOf(error) === 404
            ? t("livestockCreditLines.detail.notFound")
            : getErrorMessage(error, t("livestockCreditLines.detail.loadError"))}
        </Alert>
      </Box>
    );
  }

  const empty = t("livestockCreditLines.detail.empty");
  const f = (key: string) => t(`livestockCreditLines.detail.fields.${key}`);

  const field = (label: string, value: React.ReactNode, mono = false) => (
    <Grid item xs={12} sm={6} md={4}>
      <Typography variant="caption" color="text.secondary" display="block">
        {label}
      </Typography>
      <Typography
        variant="body1"
        fontWeight={600}
        sx={{ wordBreak: "break-all", ...(mono ? { fontFamily: "monospace", fontSize: 14 } : {}) }}
      >
        {value === "" || value === undefined || value === null ? empty : value}
      </Typography>
    </Grid>
  );

  const section = (title: string, children: React.ReactNode) => (
    <Card>
      <CardContent>
        <Typography variant="h6" gutterBottom>
          {title}
        </Typography>
        <Grid container spacing={2}>
          {children}
        </Grid>
      </CardContent>
    </Card>
  );

  const assessment = line.assessment;
  const strategyLabel = (s: string) =>
    t(`livestockCreditLines.detail.strategies.${s}`, { defaultValue: s });

  return (
    <Box>
      {back}

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Grid container spacing={2} alignItems="center">
            <Grid item xs={12} md={7}>
              <Typography variant="overline" color="text.secondary">
                {t("livestockCreditLines.detail.title")} · {line.promissoryNoteNumber}
              </Typography>
              <Typography variant="h3" fontWeight={800} color="primary.main">
                {formatCreditAmount(line.amount, line.currency)}
              </Typography>
              <Typography variant="h6">{line.fullName}</Typography>
            </Grid>
            <Grid item xs={12} md={5}>
              <Stack spacing={1} alignItems={{ xs: "flex-start", md: "flex-end" }}>
                <StatusBadge status={line.status} size="medium" />
                <Stack direction="row" spacing={1}>
                  <Button
                    variant="contained"
                    startIcon={busy === "view" ? <CircularProgress size={16} color="inherit" /> : <OpenIcon />}
                    disabled={busy !== null}
                    onClick={viewNote}
                  >
                    {t("livestockCreditLines.detail.viewNote")}
                  </Button>
                  <Button
                    variant="outlined"
                    startIcon={busy === "download" ? <CircularProgress size={16} /> : <DownloadIcon />}
                    disabled={busy !== null}
                    onClick={downloadNote}
                  >
                    {t("livestockCreditLines.detail.downloadNote")}
                  </Button>
                  {line.clientId && (
                    <Button variant="outlined" onClick={() => navigate(`/admin/clients/${line.clientId}`)}>
                      {t("livestockCreditLines.detail.viewClient")}
                    </Button>
                  )}
                </Stack>
              </Stack>
            </Grid>
          </Grid>
        </CardContent>
      </Card>

      <Stack spacing={3}>
        <LivestockCreditPortfolio lineId={line.id} currency={line.currency} />

        {section(
          t("livestockCreditLines.detail.rancher"),
          <>
            {field(f("fullName"), line.fullName)}
            {field(f("idType"), line.idType)}
            {field(f("nationalId"), line.nationalId)}
            {field(f("mobilePhone"), line.mobilePhone)}
          </>
        )}

        {section(
          t("livestockCreditLines.detail.credit"),
          <>
            {field(f("amount"), formatCreditAmount(line.amount, line.currency))}
            {field(f("status"), <StatusBadge status={line.status} />)}
            {field(f("activatedAt"), formatDateTime(line.activatedAt))}
            {field(f("registeredAt"), formatDateTime(line.createdAt))}
            {field(f("partner"), line.partner)}
            {field(f("partnerReference"), line.partnerReference, true)}
          </>
        )}

        {section(
          t("livestockCreditLines.detail.signature"),
          <>
            {field(f("promissoryNote"), line.promissoryNoteNumber, true)}
            {field(f("signedAt"), formatDateTime(line.signedAt))}
            {field(f("size"), formatBytes(line.documentSize))}
            <Grid item xs={12} sm={6} md={4}>
              <Typography variant="caption" color="text.secondary" display="block">
                {f("hash")}
              </Typography>
              <Stack direction="row" alignItems="center" spacing={0.5}>
                <Tooltip title={line.documentSha256}>
                  <Typography variant="body1" fontWeight={600} sx={{ fontFamily: "monospace", fontSize: 14 }}>
                    {shortHash(line.documentSha256)}
                  </Typography>
                </Tooltip>
                <Tooltip title={t("livestockCreditLines.detail.copy")}>
                  <IconButton size="small" onClick={copyHash} aria-label={t("livestockCreditLines.detail.copy")}>
                    <CopyIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              </Stack>
              <Typography variant="caption" color="text.secondary">
                {t("livestockCreditLines.detail.hashHelp")}
              </Typography>
            </Grid>
            {field(f("signatureProvider"), line.signatureProvider)}
            {field(f("signatureValidationId"), line.signatureValidationId, true)}
            {field(f("signatureDocumentId"), line.signatureDocumentId, true)}
            {field(f("archiveUri"), line.partnerArchiveUri, true)}
          </>
        )}

        {assessment ? (
          section(
            t("livestockCreditLines.detail.assessment"),
            <>
              {field(f("quota"), formatCreditAmount(assessment.quota, assessment.currency))}
              {field(
                f("strategy"),
                <Chip size="small" color="primary" variant="outlined" label={strategyLabel(assessment.strategy)} />
              )}
              {assessment.strategy === "tiers"
                ? field(
                    f("tier"),
                    [assessment.tierLabel || assessment.tier, assessment.tierPolicyVersion]
                      .filter(Boolean)
                      .join(" · ")
                  )
                : field(f("activeCeiling"), assessment.activeCeiling)}
              {field(f("engineQuota"), formatCreditAmount(assessment.engineQuota, assessment.currency))}
              {field(f("score"), `${assessment.score} (${assessment.grade})`)}
              {field(f("assessedAt"), formatDateTime(assessment.createdAt))}
              <Grid item xs={12}>
                <Divider sx={{ mb: 1 }} />
                <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
                  {f("assessmentId")}: {assessment.id}
                </Typography>
              </Grid>
            </>
          )
        ) : (
          <Alert severity="warning">{t("livestockCreditLines.detail.noAssessment")}</Alert>
        )}
      </Stack>
    </Box>
  );
};

export default LivestockCreditLineDetail;
