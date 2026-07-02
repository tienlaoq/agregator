import { Star } from "lucide-react";
import { WorkspacePlaceholder } from "@/components/banya/workspace-placeholder";

export default function OwnerVenueReviewsPage() {
  return (
    <WorkspacePlaceholder
      icon={Star}
      title="Отзывы"
      description="Отзывы гостей с ответами и реакцией на негатив. Раздел подключается к review-service следующим шагом."
    />
  );
}
