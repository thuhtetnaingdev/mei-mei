import axios from "axios";
import type { InternalAxiosRequestConfig } from "axios";

type CompatAxiosRequestConfig = InternalAxiosRequestConfig & {
  _legacyRetryWithoutApiPrefix?: boolean;
};

const browserBaseUrl =
  typeof window !== "undefined" ? window.location.origin : "http://localhost:8080";

const configuredBaseUrl = (import.meta.env.VITE_API_URL ?? browserBaseUrl).replace(/\/+$/, "");
const apiBaseUrl = configuredBaseUrl.endsWith("/api")
  ? configuredBaseUrl.slice(0, -4)
  : configuredBaseUrl;

const api = axios.create({
  baseURL: apiBaseUrl
});

api.interceptors.request.use((config) => {
  const requestUrl = config.url ?? "";
  const isAbsoluteUrl = /^https?:\/\//i.test(requestUrl);
  const shouldUseApiPrefix =
    !isAbsoluteUrl &&
    requestUrl.startsWith("/") &&
    !requestUrl.startsWith("/api/") &&
    !requestUrl.startsWith("/auth/") &&
    !requestUrl.startsWith("/subscription/") &&
    !requestUrl.startsWith("/profiles/");

  if (shouldUseApiPrefix) {
    config.url = `/api${requestUrl}`;
  }

  const token = localStorage.getItem("panel_token");
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (!axios.isAxiosError(error) || !error.config) {
      return Promise.reject(error);
    }

    const config = error.config as CompatAxiosRequestConfig;
    const requestUrl = config.url ?? "";
    const shouldRetryLegacyRoute =
      error.response?.status === 404 &&
      !config._legacyRetryWithoutApiPrefix &&
      requestUrl.startsWith("/api/") &&
      !/^https?:\/\//i.test(requestUrl);

    if (!shouldRetryLegacyRoute) {
      return Promise.reject(error);
    }

    config._legacyRetryWithoutApiPrefix = true;
    config.url = requestUrl.replace(/^\/api/, "");
    return api.request(config);
  }
);

export default api;
