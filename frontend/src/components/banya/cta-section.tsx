import { Button } from "@/components/ui/button"
import { Building2, ArrowRight } from "lucide-react"
import Link from "next/link"

export function CTASection() {
  return (
    <section className="bg-primary py-16 md:py-24">
      <div className="container mx-auto px-4">
        <div className="flex flex-col items-center text-center">
          <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-primary-foreground/20">
            <Building2 className="h-8 w-8 text-primary-foreground" />
          </div>
          <h2 className="mb-4 max-w-2xl text-balance text-3xl font-bold text-primary-foreground md:text-4xl">
            Вы владелец бани или сауны?
          </h2>
          <p className="mb-8 max-w-xl text-pretty text-lg text-primary-foreground/90">
            Добавьте своё заведение на БаняГид и получайте новых клиентов каждый день. 
            Бесплатная регистрация и удобная система бронирования.
          </p>
          <div className="flex flex-col items-center gap-3">
            <Button
              size="lg"
              variant="secondary"
              asChild
              className="gap-2 bg-primary-foreground text-primary hover:bg-primary-foreground/90"
            >
              <Link href="/partner">
                Добавить заведение
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
            <p className="text-sm text-primary-foreground/90">
              Вы пар-мастер?{" "}
              <Link
                href="/partner/master"
                className="font-medium text-primary-foreground underline underline-offset-4 hover:text-primary-foreground"
              >
                Подать заявку
              </Link>
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
