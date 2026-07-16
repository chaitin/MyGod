import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router-dom"

import "./index.css"
import App from "./App.tsx"
import { ProductInfoProvider } from "@/components/product-info-provider.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { Toaster } from "@/components/ui/sonner.tsx"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <ThemeProvider>
        <ProductInfoProvider>
          <App />
        </ProductInfoProvider>
        <Toaster position="top-center" />
      </ThemeProvider>
    </BrowserRouter>
  </StrictMode>
)
