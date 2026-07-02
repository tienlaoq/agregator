import { Package } from "lucide-react";
import { WorkspacePlaceholder } from "@/components/banya/workspace-placeholder";

export default function OwnerVenueStockPage() {
  return (
    <WorkspacePlaceholder
      icon={Package}
      title="Склад · товары"
      description="Учёт расходников и товаров (веники, полотенца, чай): остатки, порог «мало», списание на бронь. Модуль в разработке."
    />
  );
}
