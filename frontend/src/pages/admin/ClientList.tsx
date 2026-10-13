import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Box,
  Typography,
  TextField,
  InputAdornment,
  Button,
} from "@mui/material";
import { Search as SearchIcon, Add as AddIcon } from "@mui/icons-material";
import { format } from "date-fns";
import { adminGetClients, adminSearchClients } from "../../api/endpoints";
import DataTable, { Column } from "../../components/DataTable";
import StatusBadge from "../../components/StatusBadge";
import RegisterClientDialog from "../../components/RegisterClientDialog";
import type { Client } from "../../api/types";
import { useCountryConfig } from "../../config/countryConfig";

const ClientList: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const config = useCountryConfig();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [registerOpen, setRegisterOpen] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(search), 300);
    return () => clearTimeout(timer);
  }, [search]);

  const { data: clientsData, isLoading } = useQuery({
    queryKey: ["admin-clients", page, pageSize],
    queryFn: () => adminGetClients(page + 1, pageSize),
    enabled: !debouncedSearch,
  });

  const { data: searchResults, isLoading: searchLoading } = useQuery({
    queryKey: ["admin-clients-search", debouncedSearch],
    queryFn: () => adminSearchClients(debouncedSearch),
    enabled: !!debouncedSearch,
  });

  const columns: Column<Client>[] = [
    {
      id: "lastName",
      label: t("common.name"),
      minWidth: 150,
      render: (row) => (
        <Typography
          variant="body2"
          sx={{ cursor: "pointer", color: "primary.main", fontWeight: 500 }}
          onClick={() => navigate(`/admin/clients/${row.id}`)}
        >
          {row.lastName}, {row.firstName}
        </Typography>
      ),
    },
    { id: "dni", label: config.nationalIdLabel, minWidth: 100 },
    { id: "email", label: "Email", minWidth: 180 },
    { id: "phone", label: t("registration.phone"), minWidth: 120 },
    {
      id: "city",
      label: t("registration.city"),
      render: (row) => `${row.city}, ${row.province}`,
    },
    {
      id: "isBlocked",
      label: t("common.status"),
      render: (row) => <StatusBadge status={row.isBlocked ? "blocked" : "active"} />,
    },
    {
      id: "createdAt",
      label: t("admin.registrationDate"),
      render: (row) => format(new Date(row.createdAt), "dd/MM/yyyy"),
    },
  ];

  const rows = debouncedSearch ? (searchResults || []) : (clientsData?.data || []);
  const loading = debouncedSearch ? searchLoading : isLoading;

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h4">{t("nav.clients")}</Typography>
        <Button variant="contained" startIcon={<AddIcon />} onClick={() => setRegisterOpen(true)}>
          {t("admin.registerClient")}
        </Button>
      </Box>

      <Box mb={3}>
        <TextField
          fullWidth
          placeholder={t("admin.searchPlaceholder")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon />
              </InputAdornment>
            ),
          }}
          sx={{ maxWidth: 500 }}
        />
      </Box>

      <DataTable
        columns={columns}
        rows={rows}
        total={debouncedSearch ? rows.length : (clientsData?.total || 0)}
        page={page}
        pageSize={pageSize}
        onPageChange={debouncedSearch ? undefined : setPage}
        onPageSizeChange={debouncedSearch ? undefined : (size) => { setPageSize(size); setPage(0); }}
        loading={loading}
        keyExtractor={(row) => row.id}
        emptyMessage={t("admin.noClientsFound")}
      />

      <RegisterClientDialog open={registerOpen} onClose={() => setRegisterOpen(false)} />
    </Box>
  );
};

export default ClientList;
