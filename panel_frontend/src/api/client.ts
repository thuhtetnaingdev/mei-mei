import axios from "axios";

const browserBaseUrl =
  typeof window !== "undefined" ? window.location.origin : "http://localhost:8080";

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL ?? browserBaseUrl
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem("panel_token");
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

export default api;
