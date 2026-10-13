import React from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useAuth } from "./AuthContext";
import { CircularProgress, Box } from "@mui/material";

type Role = "admin" | "client" | "vendor" | "advisor";

interface ProtectedRouteProps {
  children: React.ReactNode;
  /** A single role, or any of several. */
  requiredRole?: Role | Role[];
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children, requiredRole }) => {
  const { isAuthenticated, isLoading, user } = useAuth();
  const location = useLocation();

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <CircularProgress />
      </Box>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  if (requiredRole) {
    const allowed = Array.isArray(requiredRole) ? requiredRole : [requiredRole];
    if (!user || !allowed.includes(user.role as Role)) {
      const redirectPath =
        user?.role === "admin"
          ? "/admin/dashboard"
          : user?.role === "advisor"
            ? "/admin/livestock-quota-lookup"
            : user?.role === "vendor"
              ? "/vendor/dashboard"
              : "/dashboard";
      return <Navigate to={redirectPath} replace />;
    }
  }

  return <>{children}</>;
};

export default ProtectedRoute;
