import * as React from "react"
import { Search, UserPlus } from "lucide-react"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { useClientData } from "@/lib/client-data-context"
import type {
  ClientConversation,
  ClientConversationMember,
  ClientUser,
  ContactApp,
  ContactUser,
} from "@/lib/client-data-api"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

type AddGroupMembersDialogProps = {
  conversation: ClientConversation
}

type AddGroupUserCandidate = Pick<
  ContactUser,
  "avatar" | "email" | "id" | "name" | "nickname" | "phone" | "type"
>

type AddGroupAppCandidate = Pick<
  ContactApp,
  "avatar" | "description" | "id" | "name" | "type"
>

type AddGroupMemberCandidate = AddGroupUserCandidate | AddGroupAppCandidate

export function AddGroupMembersDialog({
  conversation,
}: AddGroupMembersDialogProps) {
  const { addGroupConversationMembers, contactApps, contacts, me } =
    useClientData()
  const [keyword, setKeyword] = React.useState("")
  const [open, setOpen] = React.useState(false)
  const [tab, setTab] = React.useState<"users" | "apps">("users")
  const [submitting, setSubmitting] = React.useState(false)
  const currentMember = (conversation.members ?? []).find(
    (member) => member.type === "user" && member.id === me.id
  )
  const canInviteApps =
    currentMember?.role === "owner" || currentMember?.role === "admin"
  const activeTab = canInviteApps ? tab : "users"
  const existingMemberKeys = React.useMemo(
    () =>
      new Set(
        (conversation.members ?? []).map((member) =>
          memberCandidateKey(member.type, member.id)
        )
      ),
    [conversation.members]
  )
  const [selectedMemberKeys, setSelectedMemberKeys] = React.useState<
    Set<string>
  >(() => new Set(existingMemberKeys))
  const userCandidates = React.useMemo(
    () => createUserCandidates(conversation.members ?? [], me, contacts),
    [contacts, conversation.members, me]
  )
  const appCandidates = React.useMemo(
    () => createAppCandidates(conversation.members ?? [], contactApps),
    [contactApps, conversation.members]
  )
  const filteredUserCandidates = React.useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase()

    if (!normalizedKeyword) {
      return userCandidates
    }

    return userCandidates.filter((candidate) =>
      [
        candidate.email,
        candidate.name,
        candidate.nickname,
        candidate.phone,
      ].some((value) => value.toLowerCase().includes(normalizedKeyword))
    )
  }, [keyword, userCandidates])
  const filteredAppCandidates = React.useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase()

    if (!normalizedKeyword) {
      return appCandidates
    }

    return appCandidates.filter((candidate) =>
      [candidate.name, candidate.description].some((value) =>
        value.toLowerCase().includes(normalizedKeyword)
      )
    )
  }, [appCandidates, keyword])
  const newMemberIds = React.useMemo(
    () =>
      userCandidates
        .map((candidate) => candidate.id)
        .filter((memberId) => {
          const key = memberCandidateKey("user", memberId)
          return selectedMemberKeys.has(key) && !existingMemberKeys.has(key)
        }),
    [existingMemberKeys, selectedMemberKeys, userCandidates]
  )
  const newAppIds = React.useMemo(
    () =>
      canInviteApps
        ? appCandidates
            .map((candidate) => candidate.id)
            .filter((appId) => {
              const key = memberCandidateKey("app", appId)
              return selectedMemberKeys.has(key) && !existingMemberKeys.has(key)
            })
        : [],
    [appCandidates, canInviteApps, existingMemberKeys, selectedMemberKeys]
  )
  const newMemberCount = newMemberIds.length + newAppIds.length

  function handleOpenChange(nextOpen: boolean) {
    if (submitting) {
      return
    }

    if (nextOpen) {
      setKeyword("")
      setTab("users")
      setSelectedMemberKeys(new Set(existingMemberKeys))
    }

    setOpen(nextOpen)
  }

  function toggleMember(
    candidate: AddGroupMemberCandidate,
    checked: boolean | string
  ) {
    const key = memberCandidateKey(candidate.type, candidate.id)
    if (existingMemberKeys.has(key) || submitting) {
      return
    }

    setSelectedMemberKeys((currentIds) => {
      const nextChecked = Boolean(checked)
      const currentChecked = currentIds.has(key)

      if (currentChecked === nextChecked) {
        return currentIds
      }

      const nextIds = new Set(currentIds)

      if (nextChecked) {
        nextIds.add(key)
      } else {
        nextIds.delete(key)
      }

      return nextIds
    })
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (newMemberCount === 0 || submitting) {
      return
    }

    setSubmitting(true)
    try {
      await addGroupConversationMembers(
        conversation.id,
        newMemberIds,
        newAppIds
      )
      setOpen(false)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "添加成员失败")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button
          aria-label="添加成员"
          size="icon-sm"
          title="添加成员"
          type="button"
          variant="ghost"
        >
          <UserPlus className="size-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="gap-5 sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-base">添加成员</DialogTitle>
          <DialogDescription className="sr-only">
            选择联系人添加到当前群聊
          </DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={handleSubmit}>
          <Tabs
            onValueChange={(value) =>
              setTab(value === "apps" && canInviteApps ? "apps" : "users")
            }
            value={activeTab}
          >
            <TabsList
              className={cn(
                "grid w-full",
                canInviteApps ? "grid-cols-2" : "grid-cols-1"
              )}
            >
              <TabsTrigger disabled={submitting} value="users">
                成员
              </TabsTrigger>
              {canInviteApps && (
                <TabsTrigger disabled={submitting} value="apps">
                  应用
                </TabsTrigger>
              )}
            </TabsList>
            <div className="grid gap-2">
              <Label htmlFor="add-group-member-search">
                {activeTab === "apps" ? "选择应用" : "选择成员"}
              </Label>
              <div className="relative">
                <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  className="pl-8"
                  disabled={submitting}
                  id="add-group-member-search"
                  onChange={(event) => setKeyword(event.target.value)}
                  placeholder={
                    activeTab === "apps" ? "搜索应用" : "搜索联系人"
                  }
                  type="search"
                  value={keyword}
                />
              </div>
            </div>
            <TabsContent value="users">
              <CandidateList
                candidates={filteredUserCandidates}
                emptyText="没有匹配的联系人"
                existingMemberKeys={existingMemberKeys}
                onToggle={toggleMember}
                selectedMemberKeys={selectedMemberKeys}
                submitting={submitting}
              />
            </TabsContent>
            <TabsContent value="apps">
              <CandidateList
                candidates={filteredAppCandidates}
                emptyText="没有匹配的应用"
                existingMemberKeys={existingMemberKeys}
                onToggle={toggleMember}
                selectedMemberKeys={selectedMemberKeys}
                submitting={submitting}
              />
            </TabsContent>
          </Tabs>
          <DialogFooter>
            <DialogClose asChild>
              <Button disabled={submitting} type="button" variant="outline">
                取消
              </Button>
            </DialogClose>
            <Button disabled={newMemberCount === 0 || submitting} type="submit">
              {submitting ? "添加中" : "添加"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CandidateList({
  candidates,
  emptyText,
  existingMemberKeys,
  onToggle,
  selectedMemberKeys,
  submitting,
}: {
  candidates: AddGroupMemberCandidate[]
  emptyText: string
  existingMemberKeys: Set<string>
  onToggle: (
    candidate: AddGroupMemberCandidate,
    checked: boolean | string
  ) => void
  selectedMemberKeys: Set<string>
  submitting: boolean
}) {
  return (
    <div className="h-64 overflow-y-auto rounded-md border">
      <ItemGroup
        aria-label="添加群聊成员"
        className="gap-1 p-2 has-data-[size=sm]:gap-1"
        role="group"
      >
        {candidates.map((candidate) => {
          const key = memberCandidateKey(candidate.type, candidate.id)
          const existing = existingMemberKeys.has(key)

          return (
            <AddGroupMemberItem
              candidate={candidate}
              checked={selectedMemberKeys.has(key)}
              disabled={existing || submitting}
              key={key}
              onCheckedChange={(checked) => onToggle(candidate, checked)}
            />
          )
        })}
        {candidates.length === 0 && (
          <div className="px-3 py-8 text-center text-sm text-muted-foreground">
            {emptyText}
          </div>
        )}
      </ItemGroup>
    </div>
  )
}

function AddGroupMemberItem({
  candidate,
  checked,
  disabled,
  onCheckedChange,
}: {
  candidate: AddGroupMemberCandidate
  checked: boolean
  disabled: boolean
  onCheckedChange: (checked: boolean | string) => void
}) {
  const checkboxId = `add-group-member-${candidate.type}-${candidate.id}`
  const displayName = getMemberDisplayName(candidate)

  return (
    <Item
      asChild
      className={cn(
        "px-2 py-1.5",
        disabled ? "cursor-default opacity-75" : "cursor-pointer",
        checked ? "bg-primary/10" : "hover:bg-muted"
      )}
      size="sm"
    >
      <Label htmlFor={checkboxId}>
        <ItemMedia>
          <Avatar
            className="rounded-sm bg-muted after:rounded-sm"
            data-size="sm"
          >
            {candidate.avatar && (
              <AvatarImage
                alt={displayName}
                className="rounded-sm"
                src={candidate.avatar}
              />
            )}
            <AvatarFallback className="rounded-sm">
              {getInitial(displayName)}
            </AvatarFallback>
          </Avatar>
        </ItemMedia>
        <ItemContent className="min-w-0">
          <ItemTitle className="truncate">{displayName}</ItemTitle>
        </ItemContent>
        <ItemActions>
          <Checkbox
            aria-label={displayName}
            checked={checked}
            disabled={disabled}
            id={checkboxId}
            onCheckedChange={onCheckedChange}
          />
        </ItemActions>
      </Label>
    </Item>
  )
}

function createUserCandidates(
  members: ClientConversationMember[],
  currentUser: ClientUser,
  contacts: ContactUser[]
) {
  const candidatesById = new Map<string, AddGroupUserCandidate>()

  for (const member of members) {
    if (member.type !== "user") {
      continue
    }
    candidatesById.set(member.id, {
      avatar: member.avatar,
      email: member.email,
      id: member.id,
      name: member.name,
      nickname: member.nickname,
      phone: member.phone,
      type: "user",
    })
  }

  candidatesById.set(currentUser.id, { ...currentUser, type: "user" })

  for (const contact of contacts) {
    candidatesById.set(contact.id, contact)
  }

  return Array.from(candidatesById.values())
}

function createAppCandidates(
  members: ClientConversationMember[],
  apps: ContactApp[]
) {
  const candidatesById = new Map<string, AddGroupAppCandidate>()

  for (const member of members) {
    if (member.type !== "app") {
      continue
    }
    candidatesById.set(member.id, {
      avatar: member.avatar,
      description: "",
      id: member.id,
      name: member.name,
      type: "app",
    })
  }

  for (const app of apps) {
    candidatesById.set(app.id, app)
  }

  return Array.from(candidatesById.values())
}

function getMemberDisplayName(
  member: Pick<AddGroupMemberCandidate, "name"> & { nickname?: string }
) {
  return member.nickname?.trim() || member.name.trim()
}

function memberCandidateKey(type: "user" | "app", id: string) {
  return `${type}:${id}`
}

function getInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}
