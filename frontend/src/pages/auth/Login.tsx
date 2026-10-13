import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  Divider,
  Card,
  CardContent,
  Typography,
  TextField,
  Button,
  InputAdornment,
  Tabs,
  Tab,
  Alert,
  ToggleButtonGroup,
  ToggleButton,
} from "@mui/material";
import {
  Email as EmailIcon,
  Phone as PhoneIcon,
  Google as GoogleIcon,
} from "@mui/icons-material";
import { isGoogleSignInEnabled, useGoogleSignIn } from "../../auth/useGoogleSignIn";
import { useAuth } from "../../auth/AuthContext";
import { useTranslation } from "react-i18next";
import LanguageSwitcher from "../../components/LanguageSwitcher";
import { useNotification } from "../../contexts/NotificationContext";
import { getErrorMessage } from "../../api/errorUtils";

const Login: React.FC = () => {
  const navigate = useNavigate();
  const { requestOTP, verifyOTP, googleLogin } = useAuth();
  const { t } = useTranslation();
  const { showError } = useNotification();

  const [tab, setTab] = useState(0); // 0 = client/vendor (OTP), 1 = admin (Google OAuth)
  const [channel, setChannel] = useState<"email" | "phone">("email");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [otpCode, setOtpCode] = useState("");
  const [otpSent, setOtpSent] = useState(false);
  const [loading, setLoading] = useState(false);

  const handleRequestOTP = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email) return;
    setLoading(true);
    try {
      await requestOTP(email);
      setOtpSent(true);
    } catch (err: unknown) {
      showError(
        getErrorMessage(
          err,
          t("auth.invalidCredentials")
        )
      );
    } finally {
      setLoading(false);
    }
  };

  const handleVerifyOTP = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!otpCode) return;
    setLoading(true);
    try {
      await verifyOTP(email, otpCode);
      navigate("/dashboard");
    } catch (err: unknown) {
      showError(getErrorMessage(err, t("auth.otpError")));
    } finally {
      setLoading(false);
    }
  };


  const handleGoogleToken = async (idToken: string) => {
    try {
      await googleLogin(idToken);
      navigate("/dashboard");
    } catch (err: unknown) {
      showError(getErrorMessage(err, t("auth.googleError")));
    }
  };
  const { ref: googleButtonRef } = useGoogleSignIn(handleGoogleToken);

  const resetOtpFlow = () => {
    setOtpSent(false);
    setOtpCode("");
  };

  const isLoading = loading;

  return (
    <Box
      sx={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background:
          "linear-gradient(135deg, #1565C0 0%, #0D47A1 50%, #1B5E20 100%)",
        p: 2,
      }}
    >
      <Card sx={{ maxWidth: 440, width: "100%", p: 1 }}>
        <CardContent>
          <Box textAlign="center" mb={2}>
            <Box sx={{ display: "flex", justifyContent: "flex-end" }}>
              <LanguageSwitcher />
            </Box>
            <Box
              component="img"
              src={`${import.meta.env.BASE_URL}logo-symbol.png`}
              alt=""
              sx={{ height: 72, mb: 1 }}
            />
            <Typography variant="h5" fontWeight={700} color="primary.main">
              {t("common.appName")}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {t("auth.login")}
            </Typography>
          </Box>

          {/* Client/Vendor OTP Login */}
          {!otpSent && (
            <form onSubmit={handleRequestOTP}>

              {channel === "email" ? (
                <TextField
                  fullWidth
                  label={t("auth.email")}
                  type="email"
                  margin="normal"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <EmailIcon color="action" />
                      </InputAdornment>
                    ),
                  }}
                />
              ) : (
                <TextField
                  fullWidth
                  label={t("auth.phoneNumber")}
                  type="tel"
                  margin="normal"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  placeholder="+54 11 4051 0100"
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <PhoneIcon color="action" />
                      </InputAdornment>
                    ),
                  }}
                />
              )}
              <Button
                type="submit"
                fullWidth
                variant="contained"
                size="large"
                disabled={
                  isLoading ||
                  (channel === "email" ? !email : !phone)
                }
                sx={{ mt: 2, mb: 2, py: 1.5 }}
              >
                {isLoading
                  ? channel === "phone"
                    ? t("auth.sendingSms")
                    : t("auth.sendingOtp")
                  : t("auth.requestOtp")}
              </Button>
            </form>
          )}

          {/* OTP Verification Step */}
          {otpSent && (
            <form onSubmit={handleVerifyOTP}>
              <Alert severity="info" sx={{ mb: 2 }}>
                {channel === "phone"
                  ? t("auth.otpSentPhone")
                  : t("auth.otpSent")}
              </Alert>
              <TextField
                fullWidth
                label={t("auth.enterOtp")}
                margin="normal"
                value={otpCode}
                onChange={(e) => {
                  const val = e.target.value.replace(/\D/g, "").slice(0, 6);
                  setOtpCode(val);
                }}
                inputProps={{ maxLength: 6, inputMode: "numeric" }}
              />
              <Button
                type="submit"
                fullWidth
                variant="contained"
                size="large"
                disabled={isLoading || otpCode.length !== 6}
                sx={{ mt: 2, mb: 1, py: 1.5 }}
              >
                {isLoading ? t("auth.verifyingOtp") : t("auth.verifyOtp")}
              </Button>
              <Button
                fullWidth
                variant="text"
                size="small"
                onClick={resetOtpFlow}
              >
                {t("common.back")}
              </Button>
            </form>
          )}

          {/* Admin Login (Google OAuth) */}

          {isGoogleSignInEnabled && (
            <>
              <Divider sx={{ my: 2 }}>
                <Typography variant="caption" color="text.secondary">
                  {t("auth.adminLogin")}
                </Typography>
              </Divider>
              <Box sx={{ display: "flex", justifyContent: "center" }} ref={googleButtonRef} />
            </>
          )}
        </CardContent>
      </Card>
    </Box>
  );
};

export default Login;
