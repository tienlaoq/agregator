import { BookMarked } from "lucide-react";
import { WorkspacePlaceholder } from "@/components/banya/workspace-placeholder";

export default function OwnerVenueBookingsPage() {
  return (
    <WorkspacePlaceholder
      icon={BookMarked}
      title="Брони"
      description="Полный список броней заведения с фильтрами по статусу и датам. Пока брони видны в «Сегодня» и на «Расписании» — отдельный раздел появится следующим шагом."
    />
  );
}
