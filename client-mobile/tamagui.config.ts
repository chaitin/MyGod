import { defaultConfig } from "@tamagui/config/v5"
import { animationsReactNative } from "@tamagui/config/v5-rn"
import { createTamagui } from "tamagui"

export const tamaguiConfig = createTamagui({
  ...defaultConfig,
  animations: animationsReactNative,
})

export type AppTamaguiConfig = typeof tamaguiConfig

declare module "tamagui" {
  interface TamaguiCustomConfig extends AppTamaguiConfig {}
}

export default tamaguiConfig
