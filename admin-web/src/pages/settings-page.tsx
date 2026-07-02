import { SaveIcon } from "lucide-react"
import { useEffect, useId, useState, type FormEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import {
  AdminSettingsRequestError,
  getInfoSettings,
  updateInfoSettings,
} from "@/lib/admin-settings"

export default function SettingsPage() {
  const appNameId = useId()
  const organizationNameId = useId()
  const [appName, setAppName] = useState("")
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [organizationName, setOrganizationName] = useState("")
  const isSubmitDisabled =
    isLoading ||
    isSaving ||
    appName.trim() === "" ||
    organizationName.trim() === ""

  useEffect(() => {
    let ignore = false

    async function loadSettings() {
      setIsLoading(true)

      try {
        const settings = await getInfoSettings()

        if (ignore) {
          return
        }

        setAppName(settings.appName)
        setOrganizationName(settings.organizationName)
      } catch (error) {
        if (ignore) {
          return
        }

        toast.error(
          error instanceof AdminSettingsRequestError
            ? error.message
            : "加载系统设置失败"
        )
      } finally {
        if (!ignore) {
          setIsLoading(false)
        }
      }
    }

    void loadSettings()

    return () => {
      ignore = true
    }
  }, [])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (isSubmitDisabled) {
      return
    }

    setIsSaving(true)

    try {
      const settings = await updateInfoSettings({
        appName,
        organizationName,
      })

      setAppName(settings.appName)
      setOrganizationName(settings.organizationName)
      toast.success("系统设置已保存")
    } catch (error) {
      toast.error(
        error instanceof AdminSettingsRequestError
          ? error.message
          : "保存系统设置失败"
      )
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-4 p-4 pt-0">
      <Card className="w-full lg:w-1/2">
        <CardHeader>
          <CardTitle>基础信息</CardTitle>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-6" onSubmit={handleSubmit}>
            <FieldGroup className="gap-4">
              <Field>
                <FieldLabel htmlFor={appNameId}>应用名称</FieldLabel>
                <Input
                  disabled={isLoading || isSaving}
                  id={appNameId}
                  onChange={(event) => setAppName(event.target.value)}
                  required
                  value={appName}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={organizationNameId}>组织名称</FieldLabel>
                <Input
                  disabled={isLoading || isSaving}
                  id={organizationNameId}
                  onChange={(event) => setOrganizationName(event.target.value)}
                  required
                  value={organizationName}
                />
              </Field>
            </FieldGroup>

            <div className="flex justify-end">
              <Button disabled={isSubmitDisabled} type="submit">
                {isSaving ? (
                  <Spinner data-icon="inline-start" />
                ) : (
                  <SaveIcon data-icon="inline-start" />
                )}
                保存设置
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
