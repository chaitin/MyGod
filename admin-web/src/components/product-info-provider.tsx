import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"

import {
  defaultProductName,
  getPublicProductInfo,
  normalizeProductName,
} from "@/lib/product-info"

type ProductInfoContextValue = {
  appName: string
  setAppName: (appName: string) => void
}

const ProductInfoContext = createContext<ProductInfoContextValue>({
  appName: defaultProductName,
  setAppName: () => undefined,
})

export function ProductInfoProvider({ children }: { children: ReactNode }) {
  const [appName, setAppNameState] = useState(defaultProductName)
  const latestRequestId = useRef(0)
  const setAppName = useCallback((nextAppName: string) => {
    latestRequestId.current += 1
    setAppNameState(normalizeProductName(nextAppName))
  }, [])

  useEffect(() => {
    const requestId = latestRequestId.current + 1
    latestRequestId.current = requestId

    async function loadProductInfo() {
      try {
        const info = await getPublicProductInfo()

        if (latestRequestId.current === requestId) {
          setAppNameState(info.appName)
        }
      } catch {
        if (latestRequestId.current === requestId) {
          setAppNameState(defaultProductName)
        }
      }
    }

    void loadProductInfo()

    return () => {
      if (latestRequestId.current === requestId) {
        latestRequestId.current += 1
      }
    }
  }, [])

  useEffect(() => {
    document.title = `${appName}管理面板`
  }, [appName])

  const value = useMemo(() => ({ appName, setAppName }), [appName, setAppName])

  return (
    <ProductInfoContext.Provider value={value}>
      {children}
    </ProductInfoContext.Provider>
  )
}

export function useProductInfo() {
  return useContext(ProductInfoContext)
}
