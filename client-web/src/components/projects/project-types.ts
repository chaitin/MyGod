export type ProjectMember = {
  initials: string
  name: string
  tone: string
}

export type ProjectTask = {
  assignee: ProjectMember | null
  description: string
  dueDate: string | null
  id: string
  priority: "low" | "medium" | "high"
  startDate: string | null
  status: "todo" | "in_progress" | "done" | "canceled"
  title: string
}
