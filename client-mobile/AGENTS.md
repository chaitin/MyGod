# Mobile client guidance

- The mobile client uses Expo Router and Tamagui.
- Tamagui's complete official documentation is available at https://tamagui.dev/llms.txt.
- Before changing Tamagui setup, components, themes, Metro, Babel, or web behavior, consult the current official documentation above and prefer the 2.x component sections.
- Tamagui 2.4.5 is patched for upstream issue https://github.com/tamagui/tamagui/issues/4074. Recheck and remove the dependency patch when upgrading Tamagui to a version containing the upstream fix.
- Keep route files thin. Put reusable UI in `src/components`, feature-specific code in `src/features`, shared configuration in `src/config`, and providers in `src/providers`.
- Form screens with text inputs must use the shared `src/components/layout/keyboard-aware-screen.tsx` container so keyboard avoidance and scrolling remain consistent on Android and iOS.
- Use Tamagui's official default configuration and component variants. Do not introduce a second styling system unless the user explicitly requests it.
