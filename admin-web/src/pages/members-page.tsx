import {
  type ColumnDef,
  type PaginationState,
  type RowSelectionState,
  type SortingState,
  type VisibilityState,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from "@tanstack/react-table"
import {
  ArrowUpDownIcon,
  ChevronDownIcon,
  MoreHorizontalIcon,
  PlusIcon,
} from "lucide-react"
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useState,
  type FormEvent,
} from "react"
import { toast } from "sonner"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
} from "@/components/ui/pagination"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Spinner } from "@/components/ui/spinner"
import {
  type AdminUsersSort,
  type AdminUsersSortOrder,
  AdminUserRequestError,
  createAdminUser,
  listAdminUsers,
  resetAdminUserPassword,
  type AdminUser,
  updateAdminUserStatus,
} from "@/lib/admin-users"
import { cn } from "@/lib/utils"

type Member = {
  avatar: string
  email: string
  id: string
  joinedAt: string
  name: string
  nickname: string
  phone: string
  status: "disabled" | "enabled"
}

type ResetPasswordDialogState = {
  isPending: boolean
  member: Member | null
  newPassword: string
  open: boolean
}

type MemberActionConfirmationAction = "disable" | "enable" | "reset_password"

type MemberActionConfirmationState = {
  action: MemberActionConfirmationAction
  member: Member
}

type MemberActionConfirmationCopy = {
  confirmLabel: string
  description: string
  title: string
}

const pageSizeOptions = [50, 100, 200, 500] as const
type PageSize = (typeof pageSizeOptions)[number]
type PaginationItemModel =
  | {
      page: number
      type: "page"
    }
  | {
      key: string
      type: "ellipsis"
    }

const columnLabels: Record<string, string> = {
  email: "邮箱",
  joinedAt: "加入时间",
  name: "名称",
  nickname: "昵称",
  phone: "手机号",
  status: "状态",
}

function getResetPasswordClosedDialogState(): ResetPasswordDialogState {
  return {
    isPending: false,
    member: null,
    newPassword: "",
    open: false,
  }
}

export function getResetPasswordPendingDialogState(
  member: Member
): ResetPasswordDialogState {
  return {
    isPending: true,
    member,
    newPassword: "",
    open: true,
  }
}

function getResetPasswordSuccessDialogState(
  member: Member,
  newPassword: string
): ResetPasswordDialogState {
  return {
    isPending: false,
    member,
    newPassword,
    open: true,
  }
}

export function getMemberActionConfirmation(
  action: MemberActionConfirmationAction
): MemberActionConfirmationCopy {
  switch (action) {
    case "enable":
      return {
        confirmLabel: "确认启用",
        description: "启用后，该成员可以重新登录系统。",
        title: "确认启用成员",
      }
    case "disable":
      return {
        confirmLabel: "确认禁用",
        description: "禁用后，该成员将无法继续登录，已有会话也会失效。",
        title: "确认禁用成员",
      }
    case "reset_password":
      return {
        confirmLabel: "确认重置",
        description: "重置后旧密码将失效，新密码只会显示一次。",
        title: "确认重置密码",
      }
  }
}

export function getColumns({
  onResetPassword,
  onStatusChange,
  updatingMemberId,
}: {
  onResetPassword: (member: Member) => void
  onStatusChange: (member: Member, status: Member["status"]) => void
  updatingMemberId: string | null
}): ColumnDef<Member>[] {
  return [
    {
      id: "select",
      enableHiding: false,
      enableSorting: false,
      header: ({ table }) => (
        <Checkbox
          aria-label="选择当前页全部成员"
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={
            table.getIsSomePageRowsSelected() &&
            !table.getIsAllPageRowsSelected()
          }
          onCheckedChange={(checked) =>
            table.toggleAllPageRowsSelected(checked)
          }
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          aria-label={`选择 ${row.original.name}`}
          checked={row.getIsSelected()}
          onCheckedChange={(checked) => row.toggleSelected(checked)}
        />
      ),
    },
    {
      accessorKey: "name",
      header: "名称",
      cell: ({ row }) => <MemberIdentity member={row.original} />,
    },
    {
      accessorKey: "nickname",
      header: "昵称",
      cell: ({ row }) => (
        <MemberOptionalText value={row.original.nickname} />
      ),
    },
    {
      accessorKey: "email",
      header: ({ column }) => (
        <Button
          className="-ml-2"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
          size="sm"
          variant="ghost"
        >
          邮箱
          <ArrowUpDownIcon data-icon="inline-end" />
        </Button>
      ),
      cell: ({ row }) => row.getValue("email"),
    },
    {
      accessorKey: "phone",
      header: "手机号",
      cell: ({ row }) => (
        <MemberOptionalText value={formatMemberPhone(row.original.phone)} />
      ),
    },
    {
      accessorKey: "joinedAt",
      header: ({ column }) => (
        <Button
          className="-ml-2"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
          size="sm"
          variant="ghost"
        >
          加入时间
          <ArrowUpDownIcon data-icon="inline-end" />
        </Button>
      ),
      cell: ({ row }) => row.getValue("joinedAt"),
    },
    {
      accessorKey: "status",
      header: ({ column }) => (
        <Button
          className="-ml-2"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
          size="xs"
          variant="ghost"
        >
          状态
        </Button>
      ),
      cell: ({ row }) => {
        const status = row.original.status

        return (
          <Badge
            className={cn(
              status === "enabled" &&
                "border-transparent bg-sky-500/14 text-sky-700 dark:bg-sky-400/14 dark:text-sky-300"
            )}
            variant="secondary"
          >
            {status === "enabled" ? "启用" : "禁用"}
          </Badge>
        )
      },
    },
    {
      id: "actions",
      enableHiding: false,
      enableSorting: false,
      header: () => <div className="text-right">操作</div>,
      cell: ({ row }) => (
        <div className="text-right">
          <MemberActions
            isUpdating={updatingMemberId === row.original.id}
            member={row.original}
            onResetPassword={onResetPassword}
            onStatusChange={onStatusChange}
          />
        </div>
      ),
    },
  ]
}

export default function MembersPage() {
  const resetPasswordEmailId = useId()
  const resetPasswordValueId = useId()
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [isLoadingMembers, setIsLoadingMembers] = useState(true)
  const [keyword, setKeyword] = useState("")
  const [memberTotal, setMemberTotal] = useState(0)
  const [members, setMembers] = useState<Member[]>([])
  const [membersReloadKey, setMembersReloadKey] = useState(0)
  const [memberActionConfirmation, setMemberActionConfirmation] =
    useState<MemberActionConfirmationState | null>(null)
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 50,
  })
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [resetPasswordDialog, setResetPasswordDialog] =
    useState<ResetPasswordDialogState>(getResetPasswordClosedDialogState)
  const [sorting, setSorting] = useState<SortingState>([])
  const [updatingMemberId, setUpdatingMemberId] = useState<string | null>(null)
  const handleRequestMemberStatusChange = useCallback(
    (member: Member, status: Member["status"]) => {
      setMemberActionConfirmation({
        action: status === "enabled" ? "enable" : "disable",
        member,
      })
    },
    []
  )
  const handleRequestResetMemberPassword = useCallback((member: Member) => {
    setMemberActionConfirmation({
      action: "reset_password",
      member,
    })
  }, [])
  const columns = useMemo(
    () =>
      getColumns({
        onResetPassword: handleRequestResetMemberPassword,
        onStatusChange: handleRequestMemberStatusChange,
        updatingMemberId,
      }),
    [
      handleRequestMemberStatusChange,
      handleRequestResetMemberPassword,
      updatingMemberId,
    ]
  )
  const serverPageCount = Math.max(
    1,
    Math.ceil(memberTotal / pagination.pageSize)
  )

  const table = useReactTable({
    columns,
    data: members,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualSorting: true,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPagination,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    pageCount: serverPageCount,
    state: {
      columnVisibility,
      pagination,
      rowSelection,
      sorting,
    },
  })
  const page = pagination.pageIndex + 1
  const pageCount = table.getPageCount()
  const memberActionConfirmationCopy = memberActionConfirmation
    ? getMemberActionConfirmation(memberActionConfirmation.action)
    : null
  const visiblePaginationItems = getVisiblePaginationItems(page, pageCount)

  useEffect(() => {
    let ignore = false
    const sortingParams = getAdminUsersSorting(sorting)

    async function loadMembers() {
      setIsLoadingMembers(true)

      try {
        const result = await listAdminUsers({
          keyword,
          page: pagination.pageIndex + 1,
          pageSize: pagination.pageSize,
          ...sortingParams,
        })

        if (ignore) {
          return
        }

        setMemberTotal(result.total)
        setMembers(result.users.map(toMember))
        setRowSelection({})
      } catch (error) {
        if (ignore) {
          return
        }

        setMemberTotal(0)
        setMembers([])
        toast.error(
          error instanceof AdminUserRequestError
            ? error.message
            : "加载成员失败"
        )
      } finally {
        if (!ignore) {
          setIsLoadingMembers(false)
        }
      }
    }

    void loadMembers()

    return () => {
      ignore = true
    }
  }, [
    keyword,
    membersReloadKey,
    pagination.pageIndex,
    pagination.pageSize,
    sorting,
  ])

  function setPageSize(value: string | null) {
    if (!isPageSize(value)) {
      return
    }

    setPagination({
      pageIndex: 0,
      pageSize: Number(value),
    })
  }

  function handleResetPasswordOpenChange(open: boolean) {
    if (open) {
      setResetPasswordDialog((currentDialog) => ({
        ...currentDialog,
        open: true,
      }))
      return
    }

    if (resetPasswordDialog.isPending) {
      return
    }

    setResetPasswordDialog(getResetPasswordClosedDialogState())
  }

  function handleMemberActionConfirmationOpenChange(open: boolean) {
    if (!open) {
      setMemberActionConfirmation(null)
    }
  }

  function handleMemberCreated() {
    setPagination((currentPagination) => ({
      ...currentPagination,
      pageIndex: 0,
    }))
    setMembersReloadKey((currentKey) => currentKey + 1)
  }

  async function handleMemberStatusChange(
    member: Member,
    status: Member["status"]
  ) {
    setUpdatingMemberId(member.id)

    try {
      const updatedUser = await updateAdminUserStatus(
        member.id,
        toAdminUserStatus(status)
      )
      const updatedMember = toMember(updatedUser)

      setMembers((currentMembers) =>
        currentMembers.map((currentMember) =>
          currentMember.id === updatedMember.id ? updatedMember : currentMember
        )
      )
      toast.success(status === "enabled" ? "成员已启用" : "成员已禁用")
    } catch (error) {
      toast.error(
        error instanceof AdminUserRequestError
          ? error.message
          : "更新成员状态失败"
      )
    } finally {
      setUpdatingMemberId(null)
    }
  }

  function handleConfirmMemberAction() {
    const confirmation = memberActionConfirmation

    if (!confirmation) {
      return
    }

    setMemberActionConfirmation(null)

    switch (confirmation.action) {
      case "enable":
        void handleMemberStatusChange(confirmation.member, "enabled")
        return
      case "disable":
        void handleMemberStatusChange(confirmation.member, "disabled")
        return
      case "reset_password":
        void handleResetMemberPassword(confirmation.member)
        return
    }
  }

  async function handleResetMemberPassword(member: Member) {
    setResetPasswordDialog(getResetPasswordPendingDialogState(member))
    setUpdatingMemberId(member.id)

    try {
      const result = await resetAdminUserPassword(member.id)
      const updatedMember = toMember(result.user)

      setMembers((currentMembers) =>
        currentMembers.map((currentMember) =>
          currentMember.id === updatedMember.id ? updatedMember : currentMember
        )
      )
      setResetPasswordDialog(
        getResetPasswordSuccessDialogState(updatedMember, result.newPassword)
      )
    } catch (error) {
      toast.error(
        error instanceof AdminUserRequestError ? error.message : "重置密码失败"
      )
      setResetPasswordDialog(getResetPasswordClosedDialogState())
    } finally {
      setUpdatingMemberId(null)
    }
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-4 p-4 pt-0">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <Input
          className="sm:max-w-sm"
          onChange={(event) => {
            setKeyword(event.target.value)
            setPagination((currentPagination) => ({
              ...currentPagination,
              pageIndex: 0,
            }))
          }}
          placeholder="搜索用户"
          value={keyword}
        />
        <div className="flex items-center justify-end gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger render={<Button variant="outline" />}>
              选择列
              <ChevronDownIcon data-icon="inline-end" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-40">
              <DropdownMenuGroup>
                {table
                  .getAllColumns()
                  .filter((column) => column.getCanHide())
                  .map((column) => (
                    <DropdownMenuCheckboxItem
                      checked={column.getIsVisible()}
                      key={column.id}
                      onCheckedChange={(checked) =>
                        column.toggleVisibility(checked)
                      }
                    >
                      {getColumnLabel(column.id)}
                    </DropdownMenuCheckboxItem>
                  ))}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>

          <AddMemberDialog onMemberCreated={handleMemberCreated} />

          <Dialog
            onOpenChange={handleResetPasswordOpenChange}
            open={resetPasswordDialog.open}
          >
            <DialogContent showCloseButton={!resetPasswordDialog.isPending}>
              <DialogHeader>
                <DialogTitle>重置密码</DialogTitle>
              </DialogHeader>
              <div className="flex flex-col gap-6">
                <FieldGroup className="gap-4">
                  <Field>
                    <FieldLabel htmlFor={resetPasswordEmailId}>邮箱</FieldLabel>
                    <Input
                      id={resetPasswordEmailId}
                      readOnly
                      value={resetPasswordDialog.member?.email ?? ""}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={resetPasswordValueId}>
                      新密码
                    </FieldLabel>
                    <InputGroup>
                      <InputGroupInput
                        id={resetPasswordValueId}
                        placeholder={
                          resetPasswordDialog.isPending
                            ? "正在重置密码"
                            : undefined
                        }
                        readOnly
                        value={resetPasswordDialog.newPassword}
                      />
                      {resetPasswordDialog.isPending && (
                        <InputGroupAddon align="inline-end">
                          <Spinner />
                        </InputGroupAddon>
                      )}
                    </InputGroup>
                  </Field>
                </FieldGroup>
                <DialogFooter>
                  <Button
                    disabled={resetPasswordDialog.isPending}
                    onClick={() => handleResetPasswordOpenChange(false)}
                    type="button"
                  >
                    {resetPasswordDialog.isPending && (
                      <Spinner data-icon="inline-start" />
                    )}
                    {resetPasswordDialog.isPending ? "重置中" : "关闭"}
                  </Button>
                </DialogFooter>
              </div>
            </DialogContent>
          </Dialog>

          <AlertDialog
            onOpenChange={handleMemberActionConfirmationOpenChange}
            open={memberActionConfirmation !== null}
          >
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {memberActionConfirmationCopy?.title}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {memberActionConfirmationCopy?.description}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction onClick={handleConfirmMemberAction}>
                  {memberActionConfirmationCopy?.confirmLabel}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>

      <div className={getMembersTableContainerClassName()}>
        <Table className={getMembersTableClassName()}>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead
                    className={getMemberColumnClassName(header.column.id)}
                    key={header.id}
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                          header.column.columnDef.header,
                          header.getContext()
                        )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  className={cn(
                    row.original.status === "disabled" &&
                      "text-muted-foreground"
                  )}
                  data-state={row.getIsSelected() ? "selected" : undefined}
                  key={row.id}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell
                      className={getMemberColumnClassName(cell.column.id)}
                      key={cell.id}
                    >
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext()
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  className="h-24 text-center"
                  colSpan={table.getAllColumns().length}
                >
                  {isLoadingMembers ? "加载中" : "没有结果"}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <Select onValueChange={setPageSize} value={String(pagination.pageSize)}>
          <SelectTrigger className="w-32" size="sm">
            <SelectValue>{`每页 ${pagination.pageSize} 条`}</SelectValue>
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {pageSizeOptions.map((option) => (
                <SelectItem key={option} value={String(option)}>
                  {`每页 ${option} 条`}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>

        <Pagination className="mx-0 w-auto justify-end">
          <PaginationContent>
            {visiblePaginationItems.map((item) =>
              item.type === "ellipsis" ? (
                <PaginationItem key={item.key}>
                  <PaginationEllipsis />
                </PaginationItem>
              ) : (
                <PaginationItem key={item.page}>
                  <PaginationLink
                    href="#"
                    isActive={item.page === page}
                    onClick={(event) => {
                      event.preventDefault()
                      table.setPageIndex(item.page - 1)
                    }}
                  >
                    {item.page}
                  </PaginationLink>
                </PaginationItem>
              )
            )}
          </PaginationContent>
        </Pagination>
      </div>
    </div>
  )
}

function AddMemberDialog({
  onMemberCreated,
}: {
  onMemberCreated: () => void
}) {
  const addMemberEmailId = useId()
  const addMemberNameId = useId()
  const addMemberPasswordId = useId()
  const addMemberPhoneId = useId()
  const [addMemberEmail, setAddMemberEmail] = useState("")
  const [addMemberInitialPassword, setAddMemberInitialPassword] = useState<
    string | null
  >(null)
  const [addMemberName, setAddMemberName] = useState("")
  const [addMemberOpen, setAddMemberOpen] = useState(false)
  const [addMemberPhone, setAddMemberPhone] = useState("")
  const [isAddingMember, setIsAddingMember] = useState(false)
  const isAddMemberComplete = addMemberInitialPassword !== null

  function resetAddMemberForm() {
    setAddMemberEmail("")
    setAddMemberInitialPassword(null)
    setAddMemberName("")
    setAddMemberPhone("")
  }

  function handleAddMemberOpenChange(open: boolean) {
    if (open) {
      resetAddMemberForm()
    }

    setAddMemberOpen(open)
  }

  async function handleAddMemberSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    setIsAddingMember(true)

    try {
      const created = await createAdminUser({
        email: addMemberEmail,
        name: addMemberName,
        phone: addMemberPhone,
      })

      setAddMemberInitialPassword(created.initialPassword)
      onMemberCreated()
    } catch (error) {
      toast.error(
        error instanceof AdminUserRequestError ? error.message : "添加成员失败"
      )
    } finally {
      setIsAddingMember(false)
    }
  }

  return (
    <Dialog onOpenChange={handleAddMemberOpenChange} open={addMemberOpen}>
      <DialogTrigger render={<Button />}>
        <PlusIcon data-icon="inline-start" />
        添加成员
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>添加成员</DialogTitle>
        </DialogHeader>
        <form className="flex flex-col gap-6" onSubmit={handleAddMemberSubmit}>
          <FieldGroup className="gap-4">
            <Field>
              <FieldLabel htmlFor={addMemberEmailId}>邮箱</FieldLabel>
              <Input
                autoComplete="email"
                disabled={isAddingMember}
                id={addMemberEmailId}
                onChange={(event) => setAddMemberEmail(event.target.value)}
                readOnly={isAddMemberComplete}
                required
                type="email"
                value={addMemberEmail}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={addMemberNameId}>名称</FieldLabel>
              <Input
                autoComplete="name"
                disabled={isAddingMember}
                id={addMemberNameId}
                onChange={(event) => setAddMemberName(event.target.value)}
                readOnly={isAddMemberComplete}
                required
                value={addMemberName}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={addMemberPhoneId}>手机号</FieldLabel>
              <Input
                autoComplete="tel"
                disabled={isAddingMember}
                id={addMemberPhoneId}
                onChange={(event) => setAddMemberPhone(event.target.value)}
                readOnly={isAddMemberComplete}
                value={addMemberPhone}
              />
            </Field>
            {isAddMemberComplete && (
              <Field>
                <FieldLabel htmlFor={addMemberPasswordId}>初始化密码</FieldLabel>
                <Input
                  id={addMemberPasswordId}
                  readOnly
                  value={addMemberInitialPassword}
                />
              </Field>
            )}
          </FieldGroup>
          <DialogFooter>
            {isAddMemberComplete ? (
              <Button
                onClick={() => handleAddMemberOpenChange(false)}
                type="button"
              >
                关闭
              </Button>
            ) : (
              <Button disabled={isAddingMember} type="submit">
                {isAddingMember && <Spinner data-icon="inline-start" />}
                提交
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function MemberActions({
  isUpdating,
  member,
  onResetPassword,
  onStatusChange,
}: {
  isUpdating: boolean
  member: Member
  onResetPassword: (member: Member) => void
  onStatusChange: (member: Member, status: Member["status"]) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            aria-label={`打开 ${member.name} 的操作菜单`}
            size="icon-xs"
            variant="ghost"
          />
        }
      >
        <span className="sr-only">Open menu</span>
        <MoreHorizontalIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          <DropdownMenuItem
            disabled={isUpdating || member.status === "enabled"}
            onClick={() => onStatusChange(member, "enabled")}
          >
            启用
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating || member.status === "disabled"}
            onClick={() => onStatusChange(member, "disabled")}
          >
            禁用
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isUpdating}
            onClick={() => onResetPassword(member)}
          >
            重置密码
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function MemberIdentity({ member }: { member: Member }) {
  const displayName = member.name.trim() || member.email

  return (
    <div className="flex min-w-0 items-center gap-3">
      <Avatar className={getMemberAvatarClassName()} size="sm">
        {member.avatar ? (
          <AvatarImage alt={`${displayName} 头像`} src={member.avatar} />
        ) : null}
        <AvatarFallback>{getMemberAvatarFallback(member)}</AvatarFallback>
      </Avatar>
      <div className="min-w-0">
        <div className="truncate font-medium text-foreground">
          {displayName}
        </div>
      </div>
    </div>
  )
}

function MemberOptionalText({ value }: { value: string }) {
  const displayValue = getMemberOptionalDisplayValue(value)
  const isMissing = value.trim() === ""

  return (
    <span className={cn(isMissing && "text-muted-foreground")}>
      {displayValue}
    </span>
  )
}

function getColumnLabel(columnId: string) {
  return columnLabels[columnId] ?? columnId
}

export function getMemberColumnClassName(columnId: string) {
  if (columnId === "select") {
    return "w-10"
  }

  if (columnId === "actions") {
    return "w-12"
  }

  return undefined
}

export function getMembersTableContainerClassName() {
  return "overflow-hidden rounded-lg border bg-background"
}

export function getMembersTableClassName() {
  return "[&_tr>*:first-child]:pl-4 [&_tr>*:last-child]:pr-4"
}

export function getMemberAvatarClassName() {
  return "rounded-sm after:rounded-sm [&_[data-slot=avatar-fallback]]:rounded-sm [&_[data-slot=avatar-image]]:rounded-sm"
}

function getAdminUsersSorting(sorting: SortingState): {
  order?: AdminUsersSortOrder
  sort?: AdminUsersSort
} {
  const sortingRule = sorting[0]

  if (!sortingRule) {
    return {}
  }

  const sort = toAdminUsersSort(sortingRule.id)

  if (!sort) {
    return {}
  }

  return {
    order: sortingRule.desc ? "desc" : "asc",
    sort,
  }
}

function toAdminUsersSort(columnId: string): AdminUsersSort | null {
  if (columnId === "email") {
    return "email"
  }

  if (columnId === "joinedAt") {
    return "created_at"
  }

  if (columnId === "status") {
    return "status"
  }

  return null
}

function toAdminUserStatus(status: Member["status"]): AdminUser["status"] {
  return status === "enabled" ? "active" : "disabled"
}

function toMember(user: AdminUser): Member {
  return {
    avatar: user.avatar,
    email: user.email,
    id: user.id,
    joinedAt: formatJoinedAt(user.createdAt),
    name: user.name,
    nickname: user.nickname,
    phone: user.phone,
    status: user.status === "disabled" ? "disabled" : "enabled",
  }
}

export function formatMemberPhone(phone: string) {
  const trimmedPhone = phone.trim()

  if (trimmedPhone.startsWith("+86")) {
    return trimmedPhone.slice(3)
  }

  return trimmedPhone
}

export function getMemberOptionalDisplayValue(value: string) {
  const trimmedValue = value.trim()

  return trimmedValue || "-"
}

export function getMemberAvatarFallback(
  member: Pick<Member, "email" | "name" | "nickname">
) {
  const fallbackSource =
    member.nickname.trim() || member.name.trim() || member.email.trim()
  const [initial] = Array.from(fallbackSource)

  return initial?.toUpperCase() ?? "?"
}

function formatJoinedAt(value: string) {
  const date = new Date(value)

  if (Number.isNaN(date.getTime())) {
    return value
  }

  return date.toISOString().slice(0, 10)
}

function getVisiblePaginationItems(
  page: number,
  totalPages: number
): PaginationItemModel[] {
  const pageNumbers = new Set<number>()

  for (
    let pageNumber = 1;
    pageNumber <= Math.min(3, totalPages);
    pageNumber++
  ) {
    pageNumbers.add(pageNumber)
  }

  for (
    let pageNumber = Math.max(1, page - 2);
    pageNumber <= Math.min(totalPages, page + 2);
    pageNumber++
  ) {
    pageNumbers.add(pageNumber)
  }

  for (
    let pageNumber = Math.max(1, totalPages - 2);
    pageNumber <= totalPages;
    pageNumber++
  ) {
    pageNumbers.add(pageNumber)
  }

  return Array.from(pageNumbers)
    .sort((firstPage, secondPage) => firstPage - secondPage)
    .reduce<PaginationItemModel[]>((items, pageNumber, index, pages) => {
      const previousPage = pages[index - 1]

      if (previousPage && pageNumber - previousPage > 1) {
        items.push({
          key: `ellipsis-${previousPage}-${pageNumber}`,
          type: "ellipsis",
        })
      }

      items.push({
        page: pageNumber,
        type: "page",
      })

      return items
    }, [])
}

function isPageSize(value: string | null): value is `${PageSize}` {
  return pageSizeOptions.some((option) => String(option) === value)
}
