import { BriefcaseBusiness } from "lucide-react-native"

import { FeaturePlaceholder } from "@/components/feedback/feature-placeholder"

export default function ProjectsScreen() {
  return (
    <FeaturePlaceholder
      description="项目和个人工作区会出现在这里，后续可继续加入任务列表与执行进度。"
      icon={BriefcaseBusiness}
      kicker="Workspace"
      title="让工作持续向前"
    />
  )
}
