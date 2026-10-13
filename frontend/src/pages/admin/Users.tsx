import React, { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Box,
  Typography,
  TextField,
  InputAdornment,
  Button,
  Chip,
  Paper,
  Grid,
  MenuItem,
  Select,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Tooltip,
  Alert,
} from "@mui/material";
import {
  Search as SearchIcon,
  Add as AddIcon,
  Block as DeactivateIcon,
  CheckCircle as ActivateIcon,
} from "@mui/icons-material";
import { format } from "date-fns";
import {
  adminGetUsers,
  adminCreateUser,
  adminChangeUserRole,
  adminSetUserActive,
} from "../../api/endpoints";
import DataTable, { Column } from "../../components/DataTable";
import ConfirmDialog from "../../components/ConfirmDialog";
import { useNotification } from "../../contexts/NotificationContext";
import { getErrorMessage } from "../../api/errorUtils";
import { useAuth } from "../../auth/AuthContext";
import type { AdminUser, CreateUserRequest, UserRole } from "../../api/types";

const ROLES: UserRole[] = ["admin", "client", "vendor"];

const emptyForm: CreateUserRequest = { email: "", phone: "", role: "admin" };

/**
 * Admin screen for every account in the system, whatever its role.
 *
 * Accounts are never deleted here: they carry loans, payments and audit
 * entries, so the only way out of the system is deactivation.
 */
const Users: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { user: currentUser } = useAuth();
  const { showSuccess, showError } = useNotification();

  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState<UserRole | "">("");
  const [activeFilter, setActiveFilter] = useState<"" | "true" | "false">("");
  const [createOpen, setCreateOpen] = useState(false);
  const [formData, setFormData] = useState<CreateUserRequest>(emptyForm);
  const [pendingActive, setPendingActive] = useState<AdminUser | null>(null);

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search);
      setPage(0);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-users", page, pageSize, roleFilter, activeFilter, debouncedSearch],
    queryFn: () =>
      adminGetUsers({
        role: roleFilter || undefined,
        active: activeFilter === "" ? undefined : activeFilter === "true",
        search: debouncedSearch || undefined,
        limit: pageSize,
        offset: page * pageSize,
      }),
  });

  const createMutation = useMutation({
    mutationFn: adminCreateUser,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-users"] });
      setCreateOpen(false);
      setFormData(emptyForm);
      showSuccess(t("adminUsers.userCreated"));
    },
    onError: (err: unknown) => showError(getErrorMessage(err, t("adminUsers.createError"))),
  });

  const roleMutation = useMutation({
    mutationFn: ({ id, role }: { id: string; role: UserRole }) => adminChangeUserRole(id, role),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-users"] });
      showSuccess(t("adminUsers.roleUpdated"));
    },
    onError: (err: unknown) => showError(getErrorMessage(err, t("adminUsers.roleUpdateError"))),
  });

  const activeMutation = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) => adminSetUserActive(id, active),
    onSuccess: (_result, variables) => {
      queryClient.invalidateQueries({ queryKey: ["admin-users"] });
      setPendingActive(null);
      showSuccess(variables.active ? t("adminUsers.userActivated") : t("adminUsers.userDeactivated"));
    },
    onError: (err: unknown) => {
      setPendingActive(null);
      showError(getErrorMessage(err, t("adminUsers.activeUpdateError")));
    },
  });

  const roleLabel = (role: UserRole): string => t(`adminUsers.roles.${role}`);

  const isSelf = (row: AdminUser): boolean => currentUser?.id === row.id;

  const formatDate = (value?: string): string =>
    value ? format(new Date(value), "dd/MM/yyyy HH:mm") : t("adminUsers.never");

  const columns: Column<AdminUser>[] = [
    { id: "email", label: t("adminUsers.email"), minWidth: 200 },
    {
      id: "phone",
      label: t("adminUsers.phone"),
      render: (row) => row.phone || "-",
    },
    {
      id: "role",
      label: t("adminUsers.role"),
      minWidth: 150,
      render: (row) => (
        // The backend refuses an admin changing their own role, so the control
        // is disabled on the current user's row instead of failing server-side.
        <Tooltip title={isSelf(row) ? t("adminUsers.ownRoleLocked") : ""}>
          <span>
            <Select
              size="small"
              fullWidth
              value={row.role}
              disabled={isSelf(row) || roleMutation.isPending}
              onChange={(e) => roleMutation.mutate({ id: row.id, role: e.target.value as UserRole })}
            >
              {ROLES.map((role) => (
                <MenuItem key={role} value={role}>
                  {roleLabel(role)}
                </MenuItem>
              ))}
            </Select>
          </span>
        </Tooltip>
      ),
    },
    {
      id: "isActive",
      label: t("common.status"),
      render: (row) => (
        <Chip
          size="small"
          label={row.isActive ? t("status.active") : t("status.inactive")}
          color={row.isActive ? "success" : "default"}
        />
      ),
    },
    {
      id: "lastLoginAt",
      label: t("adminUsers.lastLogin"),
      minWidth: 150,
      render: (row) => formatDate(row.lastLoginAt),
    },
    {
      id: "createdAt",
      label: t("adminUsers.createdAt"),
      minWidth: 150,
      render: (row) => formatDate(row.createdAt),
    },
    {
      id: "actions",
      label: t("common.actions"),
      align: "right",
      minWidth: 160,
      render: (row) => (
        // Deactivating your own account is refused by the backend too.
        <Tooltip title={isSelf(row) ? t("adminUsers.ownAccountLocked") : ""}>
          <span>
            <Button
              size="small"
              variant="outlined"
              color={row.isActive ? "error" : "success"}
              disabled={isSelf(row) || activeMutation.isPending}
              startIcon={row.isActive ? <DeactivateIcon /> : <ActivateIcon />}
              onClick={() => setPendingActive(row)}
            >
              {row.isActive ? t("adminUsers.deactivate") : t("adminUsers.activate")}
            </Button>
          </span>
        </Tooltip>
      ),
    },
  ];

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Box>
          <Typography variant="h4" fontWeight={700} gutterBottom>
            {t("adminUsers.title")}
          </Typography>
          <Typography color="text.secondary">{t("adminUsers.description")}</Typography>
        </Box>
        <Button variant="contained" startIcon={<AddIcon />} onClick={() => setCreateOpen(true)}>
          {t("adminUsers.createUser")}
        </Button>
      </Box>

      <Alert severity="info" sx={{ mb: 2 }}>
        {t("adminUsers.noDeleteNotice")}
      </Alert>

      <Paper sx={{ mb: 2, p: 2 }}>
        <Grid container spacing={2}>
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              size="small"
              placeholder={t("adminUsers.searchPlaceholder")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchIcon />
                  </InputAdornment>
                ),
              }}
            />
          </Grid>
          <Grid item xs={12} sm={6} md={3}>
            <TextField
              select
              fullWidth
              size="small"
              label={t("adminUsers.role")}
              value={roleFilter}
              onChange={(e) => {
                setRoleFilter(e.target.value as UserRole | "");
                setPage(0);
              }}
            >
              <MenuItem value="">{t("adminUsers.allRoles")}</MenuItem>
              {ROLES.map((role) => (
                <MenuItem key={role} value={role}>
                  {roleLabel(role)}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid item xs={12} sm={6} md={3}>
            <TextField
              select
              fullWidth
              size="small"
              label={t("common.status")}
              value={activeFilter}
              onChange={(e) => {
                setActiveFilter(e.target.value as "" | "true" | "false");
                setPage(0);
              }}
            >
              <MenuItem value="">{t("adminUsers.allStatuses")}</MenuItem>
              <MenuItem value="true">{t("status.active")}</MenuItem>
              <MenuItem value="false">{t("status.inactive")}</MenuItem>
            </TextField>
          </Grid>
        </Grid>
      </Paper>

      <DataTable
        columns={columns}
        rows={data?.users || []}
        total={data?.total || 0}
        page={page}
        pageSize={pageSize}
        onPageChange={setPage}
        onPageSizeChange={(size) => {
          setPageSize(size);
          setPage(0);
        }}
        loading={isLoading}
        emptyMessage={t("adminUsers.noUsers")}
        keyExtractor={(row) => row.id}
      />

      <Dialog open={createOpen} onClose={() => setCreateOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>{t("adminUsers.createUser")}</DialogTitle>
        <DialogContent>
          <Box display="flex" flexDirection="column" gap={2} mt={1}>
            <TextField
              label={t("adminUsers.email")}
              type="email"
              value={formData.email}
              onChange={(e) => setFormData({ ...formData, email: e.target.value })}
            />
            <TextField
              label={t("adminUsers.phone")}
              value={formData.phone}
              onChange={(e) => setFormData({ ...formData, phone: e.target.value })}
            />
            <Alert severity="info">{t("adminUsers.createAdminOnly")}</Alert>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button>
          <Button
            variant="contained"
            disabled={!formData.email || createMutation.isPending}
            onClick={() => createMutation.mutate(formData)}
          >
            {createMutation.isPending ? t("common.creating") : t("common.create")}
          </Button>
        </DialogActions>
      </Dialog>

      <ConfirmDialog
        open={!!pendingActive}
        title={pendingActive?.isActive ? t("adminUsers.deactivate") : t("adminUsers.activate")}
        message={
          pendingActive?.isActive
            ? t("adminUsers.confirmDeactivate", { email: pendingActive?.email })
            : t("adminUsers.confirmActivate", { email: pendingActive?.email })
        }
        confirmLabel={t("common.confirm")}
        cancelLabel={t("common.cancel")}
        confirmColor={pendingActive?.isActive ? "error" : "primary"}
        loading={activeMutation.isPending}
        onCancel={() => setPendingActive(null)}
        onConfirm={() => {
          if (pendingActive) {
            activeMutation.mutate({ id: pendingActive.id, active: !pendingActive.isActive });
          }
        }}
      />
    </Box>
  );
};

export default Users;
