import { Card, CardContent } from "@/components/ui/card"
import { Building2, Search, MessageSquareText } from "lucide-react"

const features = [
  {
    icon: Building2,
    title: "Бронь без звонков",
    description: "Выбор площадки, подтверждение и оплата в одном интерфейсе",
  },
  {
    icon: Search,
    title: "Удобный поиск",
    description: "Фильтры по цене, рейтингу, услугам и расположению",
  },
  {
    icon: MessageSquareText,
    title: "Проверенные отзывы",
    description: "Реальные отзывы от посетителей с подтверждённым визитом",
  },
]

export function FeaturesSection() {
  return (
    <section className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <div className="grid gap-6 md:grid-cols-3">
          {features.map((feature) => (
            <Card key={feature.title} className="border-border bg-card transition-shadow hover:shadow-lg">
              <CardContent className="flex flex-col items-center p-8 text-center">
                <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-primary/10">
                  <feature.icon className="h-7 w-7 text-primary" />
                </div>
                <h3 className="mb-2 text-xl font-semibold text-card-foreground">{feature.title}</h3>
                <p className="text-muted-foreground">{feature.description}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </section>
  )
}
