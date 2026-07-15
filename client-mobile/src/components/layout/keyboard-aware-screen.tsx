import {
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  StyleSheet,
} from "react-native"
import {
  SafeAreaView,
  type Edge,
} from "react-native-safe-area-context"
import { YStack, type YStackProps } from "tamagui"

type KeyboardAwareScreenProps = React.PropsWithChildren<
  YStackProps & {
    edges?: readonly Edge[]
    keyboardVerticalOffset?: number
    scrollable?: boolean
  }
>

export function KeyboardAwareScreen({
  children,
  edges,
  keyboardVerticalOffset = 0,
  scrollable = true,
  ...contentProps
}: KeyboardAwareScreenProps) {
  const content = (
    <YStack
      {...contentProps}
      bg="$background"
      grow={1}
      minH={0}
      shrink={scrollable ? 0 : 1}
    >
      {children}
    </YStack>
  )

  return (
    <SafeAreaView edges={edges} style={styles.fill}>
      <KeyboardAvoidingView
        behavior={Platform.select({ android: "height", ios: "padding" })}
        keyboardVerticalOffset={keyboardVerticalOffset}
        style={styles.fill}
      >
        {scrollable ? (
          <ScrollView
            contentContainerStyle={styles.scrollContent}
            keyboardDismissMode={
              Platform.OS === "ios" ? "interactive" : "on-drag"
            }
            keyboardShouldPersistTaps="handled"
          >
            {content}
          </ScrollView>
        ) : (
          content
        )}
      </KeyboardAvoidingView>
    </SafeAreaView>
  )
}

const styles = StyleSheet.create({
  fill: {
    flex: 1,
  },
  scrollContent: {
    flexGrow: 1,
  },
})
