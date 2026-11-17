import React, { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { format } from "date-fns";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  Grid,
  IconButton,
  InputLabel,
  MenuItem,
  Radio,
  RadioGroup,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  Block as BlockIcon,
  CheckCircleOutline as UnblockIcon,
  KeyboardArrowDown as ExpandIcon,
  KeyboardArrowUp as CollapseIcon,
  Link as LinkIcon,
  Payments as PayIcon,
  Sync as SyncIcon,
} from "@mui/icons-material";
import {
  adminGetLivestockCreditPortfolio,
  adminGetOdooBanks,
  adminLinkOdooPartner,
  adminRegisterLivestockCreditPayment,
  adminSetLivestockCreditBlock,
  adminSyncLivestockCreditLine,
} from "../../api/endpoints";
import { getErrorMessage } from "../../api/errorUtils";
import { useNotification } from "../../contexts/NotificationContext";
import type {
  LivestockCreditInvoice,
  LivestockCreditPortfolio as Portfolio,
  LivestockCreditPaymentResult,
} from "../../api/types";
import { formatCreditAmount, formatDate, formatDateTime } from "../../utils/creditLineFormat";

const P = "livestockCreditLines.portfolio";

/** Codes the backend answers that have their own text. */
const KNOWN_ERRORS = new Set([
  "odoo_disabled",
  "odoo_not_linked",
  "odoo_unavailable",
  "odoo_rate_limited",
  "odoo_auth_failed",
  "odoo_rejected",
  "reference_already_used",
  "amount_exceeds_residual",
  "invalid_bank",
  "invoice_of_another_customer",
  "invoice_not_found",
  "invoice_required",
  "future_date",
  "invalid_date",
  "invalid_amount",
  "invalid_reference",
  "reference_required",
  "bank_required",
  "reason_required",
  "invalid_reason",
  "document_mismatch",
  "odoo_customer_not_found",
  "already_linked",
]);

const errorData = (error: unknown) =>
  (error as { response?: { data?: { code?: string; error?: string; odooCode?: string } } })?.response?.data;

/** "YYYY-MM-DD" from Odoo, shown as dd/MM/yyyy without time zone shifts. */
const formatOdooDate = (date?: string): string => {
  if (!date) return "";
  const [y, m, d] = date.split("-");
  return y && m && d ? `${d}/${m}/${y}` : date;
};

interface Props {
  lineId: string;
  currency: string;
}

const LivestockCreditPortfolio: React.FC<Props> = ({ lineId, currency }) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { showError, showSuccess, showWarning } = useNotification();

  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const [payOpen, setPayOpen] = useState(false);
  const [blockOpen, setBlockOpen] = useState(false);
  const [candidate, setCandidate] = useState<string>("");

  const key = ["admin-livestock-credit-portfolio", lineId];
  const { data: portfolio, isLoading, error } = useQuery({
    queryKey: key,
    queryFn: () => adminGetLivestockCreditPortfolio(lineId),
  });

  const money = (raw: string) => formatCreditAmount(raw, currency);

  const errorMessage = (err: unknown, fallback: string): string => {
    const data = errorData(err);
    if (data?.code && KNOWN_ERRORS.has(data.code)) {
      return t(`${P}.errors.${data.code}`, { message: data.error ?? "", odooCode: data.odooCode ?? "" });
    }
    return getErrorMessage(err, fallback);
  };

  const store = (p: Portfolio) => {
    queryClient.setQueryData(key, p);
    queryClient.invalidateQueries({ queryKey: ["admin-livestock-credit-lines"] });
  };

  const sync = useMutation({
    mutationFn: () => adminSyncLivestockCreditLine(lineId),
    onSuccess: (p) => {
      store(p);
      showSuccess(t(`${P}.synced`));
    },
    onError: (err) => {
      queryClient.invalidateQueries({ queryKey: key });
      showError(errorMessage(err, t(`${P}.syncError`)));
    },
  });

  const link = useMutation({
    mutationFn: (partnerId: number) => adminLinkOdooPartner(lineId, partnerId),
    onSuccess: (p) => {
      store(p);
      showSuccess(t(`${P}.linked`));
    },
    onError: (err) => showError(errorMessage(err, t(`${P}.linkError`))),
  });

  if (isLoading) {
    return (
      <Card>
        <CardContent>
          <Box display="flex" justifyContent="center" py={3}>
            <CircularProgress size={28} />
          </Box>
        </CardContent>
      </Card>
    );
  }
  if (error || !portfolio) {
    return <Alert severity="error">{getErrorMessage(error, t(`${P}.loadError`))}</Alert>;
  }

  const odoo = portfolio.odoo;
  const credit = odoo.credit;
  const linked = odoo.linkStatus === "linked";
  const canAct = odoo.enabled && linked;
  const openInvoices = portfolio.invoices.filter((i) => i.open);

  const field = (label: string, value: React.ReactNode) => (
    <Grid item xs={12} sm={6} md={4}>
      <Typography variant="caption" color="text.secondary" display="block">
        {label}
      </Typography>
      <Typography variant="body1" fontWeight={600} component="div">
        {value === "" || value === undefined || value === null ? t("livestockCreditLines.detail.empty") : value}
      </Typography>
    </Grid>
  );

  const linkChip = (
    <Chip
      size="small"
      variant="outlined"
      color={linked ? "success" : odoo.linkStatus === "pending" ? "default" : "warning"}
      label={t(`${P}.linkStatus.${odoo.linkStatus}`, { defaultValue: odoo.linkStatus })}
    />
  );

  return (
    <Stack spacing={3}>
      {/* Odoo card */}
      <Card>
        <CardContent>
          <Stack
            direction={{ xs: "column", sm: "row" }}
            justifyContent="space-between"
            alignItems={{ xs: "flex-start", sm: "center" }}
            spacing={1}
            mb={2}
          >
            <Stack direction="row" spacing={1} alignItems="center">
              <Typography variant="h6">{t(`${P}.title`)}</Typography>
              {linkChip}
              {linked && odoo.partnerId ? (
                <Typography variant="body2" color="text.secondary" sx={{ fontFamily: "monospace" }}>
                  {t(`${P}.partnerId`, { id: odoo.partnerId })}
                </Typography>
              ) : null}
            </Stack>
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <Button
                variant="outlined"
                startIcon={sync.isPending ? <CircularProgress size={16} /> : <SyncIcon />}
                disabled={!odoo.enabled || sync.isPending}
                onClick={() => sync.mutate()}
              >
                {t(`${P}.sync`)}
              </Button>
              <Button
                variant="contained"
                startIcon={<PayIcon />}
                disabled={!canAct || openInvoices.length === 0}
                onClick={() => setPayOpen(true)}
              >
                {t(`${P}.registerPayment`)}
              </Button>
              <Button
                variant="outlined"
                color={credit?.blocked ? "success" : "error"}
                startIcon={credit?.blocked ? <UnblockIcon /> : <BlockIcon />}
                disabled={!canAct || !credit}
                onClick={() => setBlockOpen(true)}
              >
                {credit?.blocked ? t(`${P}.unblock`) : t(`${P}.block`)}
              </Button>
            </Stack>
          </Stack>

          {!odoo.enabled && (
            <Alert severity="info" sx={{ mb: 2 }}>
              {t(`${P}.disabled`)}
            </Alert>
          )}
          {odoo.linkStatus === "pending" && odoo.enabled && (
            <Alert severity="info" sx={{ mb: 2 }}>
              {t(`${P}.pendingHelp`)}
            </Alert>
          )}
          {odoo.linkStatus === "not_in_odoo" && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              {t(`${P}.notInOdooHelp`)}
            </Alert>
          )}
          {odoo.lastSyncError && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {t(`${P}.lastSyncError`, { message: odoo.lastSyncError, code: odoo.lastSyncErrorCode ?? "" })}
            </Alert>
          )}

          {odoo.linkStatus === "ambiguous" && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              <Typography variant="body2" gutterBottom>
                {t(`${P}.ambiguousHelp`)}
              </Typography>
              <RadioGroup value={candidate} onChange={(e) => setCandidate(e.target.value)}>
                {odoo.candidates.map((c) => (
                  <FormControlLabel
                    key={c.id}
                    value={String(c.id)}
                    control={<Radio size="small" />}
                    label={`${c.id} · ${c.name} · ${c.document}${c.city ? ` · ${c.city}` : ""}${c.state ? ` (${c.state})` : ""}`}
                  />
                ))}
              </RadioGroup>
              <Button
                size="small"
                variant="contained"
                startIcon={link.isPending ? <CircularProgress size={14} color="inherit" /> : <LinkIcon />}
                disabled={!candidate || !odoo.enabled || link.isPending}
                onClick={() => link.mutate(Number(candidate))}
                sx={{ mt: 1 }}
              >
                {t(`${P}.link`)}
              </Button>
            </Alert>
          )}

          {credit ? (
            <Grid container spacing={2}>
              {field(t(`${P}.fields.approved`), money(credit.approvedAmount))}
              {field(t(`${P}.fields.used`), money(credit.usedAmount))}
              {field(
                t(`${P}.fields.available`),
                <Typography variant="body1" fontWeight={700} color="primary.main">
                  {money(credit.availableAmount)}
                </Typography>
              )}
              {field(
                t(`${P}.fields.overdueDays`),
                credit.overdueDays > 0 ? (
                  <Chip size="small" color="error" label={t(`${P}.overdue`, { count: credit.overdueDays })} />
                ) : (
                  <Chip size="small" color="success" variant="outlined" label={t(`${P}.upToDate`)} />
                )
              )}
              {field(
                t(`${P}.fields.canBuy`),
                <Stack spacing={0.5}>
                  <Chip
                    size="small"
                    color={credit.canBuy ? "success" : "warning"}
                    label={credit.canBuy ? t(`${P}.canBuyYes`) : t(`${P}.canBuyNo`)}
                    sx={{ alignSelf: "flex-start" }}
                  />
                  {!credit.canBuy && credit.blockReason ? (
                    <Typography variant="body2" color="text.secondary">
                      {credit.blockReason}
                    </Typography>
                  ) : null}
                </Stack>
              )}
              {field(
                t(`${P}.fields.blocked`),
                credit.blocked ? (
                  <Chip size="small" color="error" icon={<BlockIcon />} label={t(`${P}.blockedYes`)} />
                ) : (
                  t(`${P}.blockedNo`)
                )
              )}
              {field(t(`${P}.fields.promissoryNoteDate`), formatOdooDate(credit.promissoryNoteDate))}
              {field(t(`${P}.fields.lastSync`), formatDateTime(odoo.lastSyncAt))}
              {field(t(`${P}.fields.snapshotAt`), formatDateTime(credit.snapshotAt))}
            </Grid>
          ) : linked ? (
            <Typography color="text.secondary">{t(`${P}.noSnapshot`)}</Typography>
          ) : null}
          {credit && (
            <Typography variant="caption" color="text.secondary" display="block" mt={2}>
              {t(`${P}.computedByOdoo`)}
            </Typography>
          )}
        </CardContent>
      </Card>

      {/* Invoices */}
      <Card>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            {t(`${P}.invoices.title`)}
          </Typography>
          {portfolio.invoices.length === 0 ? (
            <Typography color="text.secondary">{t(`${P}.invoices.empty`)}</Typography>
          ) : (
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell />
                    <TableCell>{t(`${P}.invoices.number`)}</TableCell>
                    <TableCell>{t(`${P}.invoices.date`)}</TableCell>
                    <TableCell>{t(`${P}.invoices.nextDue`)}</TableCell>
                    <TableCell>{t(`${P}.invoices.due`)}</TableCell>
                    <TableCell align="right">{t(`${P}.invoices.total`)}</TableCell>
                    <TableCell align="right">{t(`${P}.invoices.residual`)}</TableCell>
                    <TableCell>{t(`${P}.invoices.state`)}</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {portfolio.invoices.map((inv) => (
                    <React.Fragment key={inv.odooInvoiceId}>
                      <TableRow hover>
                        <TableCell padding="checkbox">
                          <IconButton
                            size="small"
                            aria-label={t(`${P}.invoices.installments`)}
                            onClick={() =>
                              setExpanded((e) => ({ ...e, [inv.odooInvoiceId]: !e[inv.odooInvoiceId] }))
                            }
                          >
                            {expanded[inv.odooInvoiceId] ? <CollapseIcon /> : <ExpandIcon />}
                          </IconButton>
                        </TableCell>
                        <TableCell sx={{ fontFamily: "monospace" }}>
                          {inv.number}
                          {portfolio.suggestedInvoiceId === inv.odooInvoiceId ? (
                            <Chip size="small" color="primary" variant="outlined" label={t(`${P}.invoices.next`)} sx={{ ml: 1 }} />
                          ) : null}
                        </TableCell>
                        <TableCell>{formatOdooDate(inv.date)}</TableCell>
                        <TableCell>{inv.open ? formatOdooDate(inv.nextDueDate) : ""}</TableCell>
                        <TableCell>{formatOdooDate(inv.dueDate)}</TableCell>
                        <TableCell align="right">{money(inv.amountTotal)}</TableCell>
                        <TableCell align="right">
                          <Typography variant="body2" fontWeight={inv.open ? 700 : 400}>
                            {money(inv.amountResidual)}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Chip
                            size="small"
                            variant="outlined"
                            color={inv.paymentState === "paid" ? "success" : inv.paymentState === "partial" ? "info" : "default"}
                            label={t(`${P}.paymentStates.${inv.paymentState}`, { defaultValue: inv.paymentState })}
                          />
                        </TableCell>
                      </TableRow>
                      <TableRow>
                        <TableCell colSpan={8} sx={{ py: 0, borderBottom: expanded[inv.odooInvoiceId] ? undefined : "none" }}>
                          <Collapse in={!!expanded[inv.odooInvoiceId]} unmountOnExit>
                            <Box py={1} pl={6}>
                              <Table size="small">
                                <TableHead>
                                  <TableRow>
                                    <TableCell>{t(`${P}.invoices.installment`)}</TableCell>
                                    <TableCell>{t(`${P}.invoices.due`)}</TableCell>
                                    <TableCell align="right">{t(`${P}.invoices.amount`)}</TableCell>
                                    <TableCell align="right">{t(`${P}.invoices.residual`)}</TableCell>
                                  </TableRow>
                                </TableHead>
                                <TableBody>
                                  {inv.installments.map((inst) => (
                                    <TableRow key={inst.number}>
                                      <TableCell>{inst.number}</TableCell>
                                      <TableCell>{formatOdooDate(inst.dueDate)}</TableCell>
                                      <TableCell align="right">{money(inst.amount)}</TableCell>
                                      <TableCell align="right">{money(inst.residual)}</TableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            </Box>
                          </Collapse>
                        </TableCell>
                      </TableRow>
                    </React.Fragment>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      {/* Payments */}
      <Card>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            {t(`${P}.payments.title`)}
          </Typography>
          {portfolio.payments.length === 0 ? (
            <Typography color="text.secondary">{t(`${P}.payments.empty`)}</Typography>
          ) : (
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>{t(`${P}.payments.date`)}</TableCell>
                    <TableCell>{t(`${P}.payments.invoice`)}</TableCell>
                    <TableCell align="right">{t(`${P}.payments.amount`)}</TableCell>
                    <TableCell>{t(`${P}.payments.bank`)}</TableCell>
                    <TableCell>{t(`${P}.payments.reference`)}</TableCell>
                    <TableCell>{t(`${P}.payments.status`)}</TableCell>
                    <TableCell>{t(`${P}.payments.odooPayment`)}</TableCell>
                    <TableCell>{t(`${P}.payments.registeredBy`)}</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {portfolio.payments.map((p) => (
                    <TableRow key={p.id}>
                      <TableCell>{formatOdooDate(p.date)}</TableCell>
                      <TableCell sx={{ fontFamily: "monospace" }}>{p.invoiceNumber}</TableCell>
                      <TableCell align="right">{money(p.amount)}</TableCell>
                      <TableCell>{p.bank}</TableCell>
                      <TableCell sx={{ fontFamily: "monospace" }}>{p.reference}</TableCell>
                      <TableCell>
                        <Tooltip title={p.errorMessage ?? ""}>
                          <Chip
                            size="small"
                            color={p.status === "registered" ? "success" : p.status === "failed" ? "error" : "warning"}
                            label={t(`${P}.paymentStatus.${p.status}`, { defaultValue: p.status })}
                          />
                        </Tooltip>
                      </TableCell>
                      <TableCell sx={{ fontFamily: "monospace" }}>{p.odooPaymentNumber || (p.errorCode ?? "")}</TableCell>
                      <TableCell>
                        <Typography variant="body2">{p.registeredBy}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          {formatDateTime(p.createdAt)}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      {/* Block history */}
      <Card>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            {t(`${P}.blocks.title`)}
          </Typography>
          {portfolio.blockEvents.length === 0 ? (
            <Typography color="text.secondary">{t(`${P}.blocks.empty`)}</Typography>
          ) : (
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>{t(`${P}.blocks.when`)}</TableCell>
                    <TableCell>{t(`${P}.blocks.action`)}</TableCell>
                    <TableCell>{t(`${P}.blocks.reason`)}</TableCell>
                    <TableCell>{t(`${P}.blocks.by`)}</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {portfolio.blockEvents.map((e) => (
                    <TableRow key={e.id}>
                      <TableCell>{formatDateTime(e.createdAt)}</TableCell>
                      <TableCell>
                        <Chip
                          size="small"
                          color={e.blocked ? "error" : "success"}
                          variant="outlined"
                          label={e.blocked ? t(`${P}.blocks.blocked`) : t(`${P}.blocks.unblocked`)}
                        />
                      </TableCell>
                      <TableCell>{e.reason}</TableCell>
                      <TableCell>{e.by}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      {payOpen && (
        <PaymentDialog
          lineId={lineId}
          invoices={openInvoices}
          suggestedInvoiceId={portfolio.suggestedInvoiceId}
          money={money}
          errorMessage={errorMessage}
          onClose={() => setPayOpen(false)}
          onDone={(res) => {
            store(res.portfolio);
            setPayOpen(false);
            if (res.replayed) showWarning(t(`${P}.pay.replayed`));
            else if (res.payment.status === "already_registered") showWarning(t(`${P}.pay.alreadyRegistered`));
            else showSuccess(t(`${P}.pay.registered`, { number: res.payment.odooPaymentNumber }));
          }}
          onFailed={() => queryClient.invalidateQueries({ queryKey: key })}
        />
      )}

      {blockOpen && credit && (
        <BlockDialog
          lineId={lineId}
          blocked={!credit.blocked}
          errorMessage={errorMessage}
          onClose={() => setBlockOpen(false)}
          onDone={(p, blocked) => {
            store(p);
            setBlockOpen(false);
            showSuccess(blocked ? t(`${P}.blockDialog.blockedDone`) : t(`${P}.blockDialog.unblockedDone`));
          }}
        />
      )}
    </Stack>
  );
};

interface PaymentDialogProps {
  lineId: string;
  invoices: LivestockCreditInvoice[];
  suggestedInvoiceId?: number;
  money: (raw: string) => string;
  errorMessage: (err: unknown, fallback: string) => string;
  onClose: () => void;
  onDone: (res: LivestockCreditPaymentResult) => void;
  onFailed: () => void;
}

/**
 * Registers one payment. Invoices come ordered by the due date of their
 * oldest unpaid installment and the oldest is preselected. The residual shown
 * is Odoo's; the backend checks it again against Odoo before sending.
 */
const PaymentDialog: React.FC<PaymentDialogProps> = ({
  lineId,
  invoices,
  suggestedInvoiceId,
  money,
  errorMessage,
  onClose,
  onDone,
  onFailed,
}) => {
  const { t } = useTranslation();
  const today = format(new Date(), "yyyy-MM-dd");
  const [invoiceId, setInvoiceId] = useState<number | "">(suggestedInvoiceId ?? invoices[0]?.odooInvoiceId ?? "");
  const [bank, setBank] = useState("");
  const [amount, setAmount] = useState("");
  const [date, setDate] = useState(today);
  const [reference, setReference] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const banks = useQuery({ queryKey: ["admin-odoo-banks"], queryFn: adminGetOdooBanks, staleTime: 5 * 60_000 });
  const invoice = useMemo(() => invoices.find((i) => i.odooInvoiceId === invoiceId), [invoices, invoiceId]);

  const amountValue = Number.parseFloat(amount.replace(",", "."));
  const overResidual = invoice && Number.isFinite(amountValue) && amountValue > Number.parseFloat(invoice.amountResidual);
  const invalidAmount = amount !== "" && (!Number.isFinite(amountValue) || amountValue <= 0 || !/^\d+([.,]\d{1,2})?$/.test(amount.trim()));
  const futureDate = date > today;
  const valid =
    invoiceId !== "" && bank && reference.trim() && amount && !invalidAmount && !overResidual && date && !futureDate;

  const pay = useMutation({
    mutationFn: () =>
      adminRegisterLivestockCreditPayment(lineId, {
        invoiceId: Number(invoiceId),
        amount: amount.trim().replace(",", "."),
        date,
        bank,
        reference: reference.trim(),
      }),
    onSuccess: onDone,
    onError: (err) => {
      setFormError(errorMessage(err, t(`${P}.pay.error`)));
      onFailed();
    },
  });

  return (
    <Dialog open onClose={pay.isPending ? undefined : onClose} fullWidth maxWidth="sm">
      <DialogTitle>{t(`${P}.pay.title`)}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} mt={1}>
          {formError && <Alert severity="error">{formError}</Alert>}
          <FormControl fullWidth required>
            <InputLabel id="pay-invoice">{t(`${P}.pay.invoice`)}</InputLabel>
            <Select
              labelId="pay-invoice"
              label={t(`${P}.pay.invoice`)}
              value={invoiceId}
              onChange={(e) => setInvoiceId(e.target.value as number)}
            >
              {invoices.map((inv) => (
                <MenuItem key={inv.odooInvoiceId} value={inv.odooInvoiceId}>
                  {t(`${P}.pay.invoiceOption`, {
                    number: inv.number,
                    due: formatOdooDate(inv.nextDueDate),
                    residual: money(inv.amountResidual),
                  })}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl fullWidth required disabled={banks.isLoading}>
            <InputLabel id="pay-bank">{t(`${P}.pay.bank`)}</InputLabel>
            <Select labelId="pay-bank" label={t(`${P}.pay.bank`)} value={bank} onChange={(e) => setBank(e.target.value)}>
              {(banks.data ?? []).map((b) => (
                <MenuItem key={b.bank} value={b.bank}>
                  {b.journal ? `${b.journal} (${b.bank})` : b.bank}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          {banks.error ? <Alert severity="warning">{errorMessage(banks.error, t(`${P}.pay.banksError`))}</Alert> : null}
          <TextField
            label={t(`${P}.pay.amount`)}
            required
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            inputProps={{ inputMode: "decimal" }}
            error={invalidAmount || !!overResidual}
            helperText={
              invalidAmount
                ? t(`${P}.pay.amountInvalid`)
                : overResidual
                  ? t(`${P}.pay.amountOver`, { residual: money(invoice!.amountResidual) })
                  : invoice
                    ? t(`${P}.pay.residualHint`, { residual: money(invoice.amountResidual) })
                    : ""
            }
          />
          <TextField
            label={t(`${P}.pay.date`)}
            type="date"
            required
            value={date}
            onChange={(e) => setDate(e.target.value)}
            InputLabelProps={{ shrink: true }}
            inputProps={{ max: today }}
            error={futureDate}
            helperText={futureDate ? t(`${P}.errors.future_date`) : ""}
          />
          <TextField
            label={t(`${P}.pay.reference`)}
            required
            value={reference}
            onChange={(e) => setReference(e.target.value)}
            inputProps={{ maxLength: 100 }}
            helperText={t(`${P}.pay.referenceHelp`)}
          />
          <Typography variant="caption" color="text.secondary">
            {t(`${P}.pay.oldestInstallment`)}
          </Typography>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={pay.isPending}>
          {t(`${P}.cancel`)}
        </Button>
        <Button
          variant="contained"
          disabled={!valid || pay.isPending}
          startIcon={pay.isPending ? <CircularProgress size={16} color="inherit" /> : <PayIcon />}
          onClick={() => {
            setFormError(null);
            pay.mutate();
          }}
        >
          {t(`${P}.pay.submit`)}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

interface BlockDialogProps {
  lineId: string;
  /** What the dialog does: block (true) or unblock (false). */
  blocked: boolean;
  errorMessage: (err: unknown, fallback: string) => string;
  onClose: () => void;
  onDone: (p: Portfolio, blocked: boolean) => void;
}

const BlockDialog: React.FC<BlockDialogProps> = ({ lineId, blocked, errorMessage, onClose, onDone }) => {
  const { t } = useTranslation();
  const [reason, setReason] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => adminSetLivestockCreditBlock(lineId, blocked, reason.trim()),
    onSuccess: (p) => onDone(p, blocked),
    onError: (err) => setFormError(errorMessage(err, t(`${P}.blockDialog.error`))),
  });
  return (
    <Dialog open onClose={mutation.isPending ? undefined : onClose} fullWidth maxWidth="sm">
      <DialogTitle>{blocked ? t(`${P}.blockDialog.blockTitle`) : t(`${P}.blockDialog.unblockTitle`)}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} mt={1}>
          {formError && <Alert severity="error">{formError}</Alert>}
          <Typography variant="body2" color="text.secondary">
            {blocked ? t(`${P}.blockDialog.blockHelp`) : t(`${P}.blockDialog.unblockHelp`)}
          </Typography>
          <TextField
            label={t(`${P}.blockDialog.reason`)}
            required
            multiline
            minRows={3}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            inputProps={{ maxLength: 500 }}
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={mutation.isPending}>
          {t(`${P}.cancel`)}
        </Button>
        <Button
          variant="contained"
          color={blocked ? "error" : "success"}
          disabled={!reason.trim() || mutation.isPending}
          startIcon={mutation.isPending ? <CircularProgress size={16} color="inherit" /> : blocked ? <BlockIcon /> : <UnblockIcon />}
          onClick={() => {
            setFormError(null);
            mutation.mutate();
          }}
        >
          {blocked ? t(`${P}.block`) : t(`${P}.unblock`)}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default LivestockCreditPortfolio;
