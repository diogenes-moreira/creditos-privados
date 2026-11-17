import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Alert, Box, Button, Chip, Grid, InputAdornment, Paper, Stack, TextField, Typography } from "@mui/material";
import { Search as SearchIcon } from "@mui/icons-material";
import { adminGetLivestockCreditLines } from "../../api/endpoints";
import DataTable, { Column } from "../../components/DataTable";
import StatusBadge from "../../components/StatusBadge";
import { getErrorMessage } from "../../api/errorUtils";
import type { LivestockCreditLine } from "../../api/types";
import { formatCreditAmount, formatDateTime } from "../../utils/creditLineFormat";

/**
 * The credit lines CuppoGan activated on a livestock quota, read-only, for
 * advisors and administrators. Filtering is by exact national ID, sent in the
 * request body so it stays out of URLs.
 */
const LivestockCreditLines: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [input, setInput] = useState("");
  const [nationalId, setNationalId] = useState("");

  const { data, isLoading, error } = useQuery({
    queryKey: ["admin-livestock-credit-lines", nationalId, page, pageSize],
    queryFn: () =>
      adminGetLivestockCreditLines({ nationalId, offset: page * pageSize, limit: pageSize }),
  });

  const applyFilter = () => {
    setNationalId(input.trim());
    setPage(0);
  };

  const clearFilter = () => {
    setInput("");
    setNationalId("");
    setPage(0);
  };

  const open = (row: LivestockCreditLine) => navigate(`/admin/livestock-credit-lines/${row.id}`);

  const columns: Column<LivestockCreditLine>[] = [
    {
      id: "fullName",
      label: t("livestockCreditLines.columns.rancher"),
      minWidth: 180,
      render: (row) => (
        <Typography
          variant="body2"
          sx={{ cursor: "pointer", color: "primary.main", fontWeight: 500 }}
          onClick={() => open(row)}
        >
          {row.fullName}
        </Typography>
      ),
    },
    { id: "nationalId", label: t("livestockCreditLines.columns.nationalId"), minWidth: 110 },
    {
      id: "amount",
      label: t("livestockCreditLines.columns.amount"),
      align: "right",
      minWidth: 120,
      render: (row) => formatCreditAmount(row.amount, row.currency),
    },
    {
      id: "promissoryNoteNumber",
      label: t("livestockCreditLines.columns.promissoryNote"),
      minWidth: 150,
      render: (row) => (
        <Typography
          variant="body2"
          sx={{ cursor: "pointer", fontFamily: "monospace" }}
          onClick={() => open(row)}
        >
          {row.promissoryNoteNumber}
        </Typography>
      ),
    },
    {
      id: "signedAt",
      label: t("livestockCreditLines.columns.signedAt"),
      minWidth: 130,
      render: (row) => formatDateTime(row.signedAt),
    },
    {
      id: "activatedAt",
      label: t("livestockCreditLines.columns.activatedAt"),
      minWidth: 130,
      render: (row) => formatDateTime(row.activatedAt),
    },
    {
      id: "status",
      label: t("livestockCreditLines.columns.status"),
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      id: "portfolio",
      label: t("livestockCreditLines.columns.portfolio"),
      minWidth: 150,
      render: (row) => <PortfolioBadges line={row} />,
    },
  ];

  return (
    <Box>
      <Box mb={3}>
        <Typography variant="h4" fontWeight={700} gutterBottom>
          {t("livestockCreditLines.title")}
        </Typography>
        <Typography color="text.secondary">{t("livestockCreditLines.subtitle")}</Typography>
      </Box>

      <Paper sx={{ p: 2, mb: 3 }}>
        <Grid container spacing={2} alignItems="center">
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              size="small"
              label={t("livestockCreditLines.filterLabel")}
              placeholder={t("livestockCreditLines.filterPlaceholder")}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") applyFilter();
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
          <Grid item xs={6} md={2}>
            <Button fullWidth variant="contained" startIcon={<SearchIcon />} onClick={applyFilter}>
              {t("livestockCreditLines.search")}
            </Button>
          </Grid>
          <Grid item xs={6} md={2}>
            <Button fullWidth variant="outlined" disabled={!input && !nationalId} onClick={clearFilter}>
              {t("livestockCreditLines.clear")}
            </Button>
          </Grid>
        </Grid>
      </Paper>

      {error ? (
        <Alert severity="error">{getErrorMessage(error, t("livestockCreditLines.loadError"))}</Alert>
      ) : (
        <DataTable
          columns={columns}
          rows={data?.data ?? []}
          total={data?.total ?? 0}
          page={page}
          pageSize={pageSize}
          onPageChange={setPage}
          onPageSizeChange={(size) => {
            setPageSize(size);
            setPage(0);
          }}
          loading={isLoading}
          keyExtractor={(row) => row.id}
          emptyMessage={
            nationalId
              ? t("livestockCreditLines.emptyFiltered", { nationalId })
              : t("livestockCreditLines.empty")
          }
        />
      )}
    </Box>
  );
};

/** Overdue days and block from the line's last Odoo snapshot. */
const PortfolioBadges: React.FC<{ line: LivestockCreditLine }> = ({ line }) => {
  const { t } = useTranslation();
  const o = line.odoo;
  if (!o || o.linkStatus !== "linked" || !o.synced) {
    const status = o?.linkStatus ?? "pending";
    return (
      <Chip
        size="small"
        variant="outlined"
        color={status === "ambiguous" || status === "not_in_odoo" ? "warning" : "default"}
        label={t(`livestockCreditLines.portfolio.linkStatus.${status}`, { defaultValue: status })}
      />
    );
  }
  return (
    <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
      {o.overdueDays > 0 ? (
        <Chip size="small" color="error" label={t("livestockCreditLines.portfolio.overdue", { count: o.overdueDays })} />
      ) : (
        <Chip size="small" color="success" variant="outlined" label={t("livestockCreditLines.portfolio.upToDate")} />
      )}
      {o.blocked && <Chip size="small" color="error" variant="outlined" label={t("livestockCreditLines.portfolio.blockedYes")} />}
    </Stack>
  );
};

export default LivestockCreditLines;
