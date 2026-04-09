"use client"

import { useState, useRef } from "react"
import { useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { register, getProfile, createVenue, ApiError } from "@/lib/api"
import { useAuthStore } from "@/store/auth"
import { CityCombobox } from "@/components/banya/city-combobox"
import { AddressSuggest } from "@/components/banya/address-suggest"
import { PhoneInput, getRawPhone } from "@/components/banya/phone-input"
import type { CreateVenueRequest } from "@/lib/types"
import Link from "next/link"
import {
  Flame,
  ArrowRight,
  ArrowLeft,
  Users,
  TrendingUp,
  CalendarCheck,
  ShieldCheck,
  Star,
  Building2,
  CheckCircle2,
  PartyPopper,
  Settings,
} from "lucide-react"

type Step = 1 | 2 | 3 | 4

export default function PartnerPage() {
  const router = useRouter()
  const authLogin = useAuthStore((s) => s.login)
  const [step, setStep] = useState<Step>(1)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  // Step 2: Venue data
  const [venueName, setVenueName] = useState("")
  const [venueType, setVenueType] = useState("")
  const [venueCity, setVenueCity] = useState("")
  const [venueAddress, setVenueAddress] = useState("")
  const [venueDescription, setVenueDescription] = useState("")

  // Step 3: Contact data
  const [contactName, setContactName] = useState("")
  const [contactEmail, setContactEmail] = useState("")
  const [contactPhone, setContactPhone] = useState("")
  const [contactPassword, setContactPassword] = useState("")

  const [createdVenueSlug, setCreatedVenueSlug] = useState("")
  const submittingRef = useRef(false)

  const handleSubmit = async () => {
    if (submittingRef.current) return
    submittingRef.current = true
    setError("")
    setLoading(true)
    try {
      const rawPhone = getRawPhone(contactPhone)
      const res = await register({
        name: contactName,
        email: contactEmail,
        phone: rawPhone,
        password: contactPassword,
        role: "venue_owner",
      })
      localStorage.setItem("token", res.access_token)
      localStorage.setItem("refresh_token", res.refresh_token)
      const user = await getProfile()
      authLogin(res.access_token, res.refresh_token, user)

      const venueData: CreateVenueRequest = {
        name: venueName,
        type: venueType as CreateVenueRequest["type"],
        city: venueCity,
        address: venueAddress,
        description: venueDescription,
        phone: rawPhone,
        price_from: 0,
        amenities: [],
        services: [],
      }
      const venue = await createVenue(venueData)
      setCreatedVenueSlug(venue.slug || venue.id)
      setStep(4)
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.message)
          setError(body.error || "Ошибка при создании аккаунта")
        } catch {
          setError(err.message)
        }
      } else {
        setError("Не удалось подключиться к серверу")
      }
    } finally {
      setLoading(false)
      submittingRef.current = false
    }
  }

  return (
    <div className="min-h-[80vh] py-12">
      {/* Progress indicator */}
      {step !== 4 && (
        <div className="container mx-auto mb-8 px-4">
          <div className="mx-auto flex max-w-2xl items-center justify-center gap-2">
            {[1, 2, 3].map((s) => (
              <div key={s} className="flex items-center gap-2">
                <div
                  className={`flex h-8 w-8 items-center justify-center rounded-full text-sm font-medium transition-colors ${
                    s === step
                      ? "bg-primary text-primary-foreground"
                      : s < step
                        ? "bg-primary/20 text-primary"
                        : "bg-muted text-muted-foreground"
                  }`}
                >
                  {s < step ? <CheckCircle2 className="h-5 w-5" /> : s}
                </div>
                <span className={`hidden text-sm sm:inline ${s === step ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                  {s === 1 ? "Выгоды" : s === 2 ? "О заведении" : "Контакты"}
                </span>
                {s < 3 && <div className={`h-px w-8 sm:w-16 ${s < step ? "bg-primary" : "bg-border"}`} />}
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="container mx-auto px-4">
        {/* Step 1: Benefits */}
        {step === 1 && (
          <div className="mx-auto max-w-3xl">
            <div className="mb-10 text-center">
              <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-primary">
                <Flame className="h-8 w-8 text-primary-foreground" />
              </div>
              <h1 className="mb-3 text-3xl font-bold text-foreground md:text-4xl">
                Развивайте бизнес с БаняГид
              </h1>
              <p className="mx-auto max-w-xl text-lg text-muted-foreground">
                Присоединяйтесь к платформе, которой доверяют сотни заведений по всей России
              </p>
            </div>

            {/* Benefits grid */}
            <div className="mb-10 grid gap-6 sm:grid-cols-2">
              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <Users className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Новые клиенты</h3>
                    <p className="text-sm text-muted-foreground">
                      Тысячи людей ищут бани каждый день. Ваше заведение увидят все.
                    </p>
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <CalendarCheck className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Онлайн-бронирование</h3>
                    <p className="text-sm text-muted-foreground">
                      Клиенты бронируют 24/7. Меньше звонков — больше посещений.
                    </p>
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <TrendingUp className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Аналитика</h3>
                    <p className="text-sm text-muted-foreground">
                      Статистика бронирований, отзывов и выручки в личном кабинете.
                    </p>
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <ShieldCheck className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Доверие и репутация</h3>
                    <p className="text-sm text-muted-foreground">
                      Верифицированные отзывы повышают доверие. В будущем — «Золотой список».
                    </p>
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Social proof */}
            <div className="mb-10 rounded-xl bg-secondary/50 p-6 text-center">
              <div className="mb-3 flex items-center justify-center gap-1">
                {[1, 2, 3, 4, 5].map((s) => (
                  <Star key={s} className="h-5 w-5 fill-amber-400 text-amber-400" />
                ))}
              </div>
              <p className="mb-2 text-muted-foreground italic">
                «За первый месяц на БаняГид мы получили 40 новых бронирований. Раньше столько набирали за квартал.»
              </p>
              <p className="text-sm font-medium text-foreground">— Алексей, владелец «Русская Банька на Дровах»</p>
            </div>

            <div className="flex justify-center">
              <Button size="lg" className="gap-2 px-10" onClick={() => setStep(2)}>
                Подать заявку
                <ArrowRight className="h-4 w-4" />
              </Button>
            </div>

            <p className="mt-4 text-center text-sm text-muted-foreground">
              Регистрация бесплатная. Без скрытых платежей.
            </p>
          </div>
        )}

        {/* Step 2: Venue Information */}
        {step === 2 && (
          <div className="mx-auto max-w-lg">
            <Card className="border-border">
              <CardHeader className="text-center">
                <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                  <Building2 className="h-6 w-6 text-primary" />
                </div>
                <CardTitle className="text-2xl text-card-foreground">Расскажите о заведении</CardTitle>
                <CardDescription>Эти данные помогут нам создать страницу вашей бани</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="venueName">Название заведения</Label>
                  <Input
                    id="venueName"
                    placeholder="Например: Русская Банька на Дровах"
                    value={venueName}
                    onChange={(e) => setVenueName(e.target.value)}
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label>Тип заведения</Label>
                  <Select value={venueType} onValueChange={setVenueType}>
                    <SelectTrigger>
                      <SelectValue placeholder="Выберите тип" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="banya">Баня</SelectItem>
                      <SelectItem value="sauna">Сауна</SelectItem>
                      <SelectItem value="hammam">Хаммам</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-2">
                    <Label>Город</Label>
                    <CityCombobox value={venueCity} onChange={setVenueCity} />
                  </div>
                  <div className="space-y-2">
                    <Label>Адрес</Label>
                    <AddressSuggest
                      value={venueAddress}
                      onChange={setVenueAddress}
                      city={venueCity}
                      placeholder="ул. Банная, д. 15"
                    />
                  </div>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="venueDescription">Описание</Label>
                  <Textarea
                    id="venueDescription"
                    placeholder="Расскажите, чем ваше заведение особенное..."
                    value={venueDescription}
                    onChange={(e) => setVenueDescription(e.target.value)}
                    rows={3}
                  />
                </div>

                <div className="flex gap-3 pt-2">
                  <Button variant="outline" className="gap-2" onClick={() => setStep(1)}>
                    <ArrowLeft className="h-4 w-4" />
                    Назад
                  </Button>
                  <Button
                    className="flex-1 gap-2"
                    onClick={() => setStep(3)}
                    disabled={!venueName || !venueType || !venueCity}
                  >
                    Далее
                    <ArrowRight className="h-4 w-4" />
                  </Button>
                </div>
              </CardContent>
            </Card>

            <p className="mt-4 text-center text-sm text-muted-foreground">
              Шаг 2 из 3 · Все данные можно изменить позже
            </p>
          </div>
        )}

        {/* Step 3: Contact Information & Account */}
        {step === 3 && (
          <div className="mx-auto max-w-lg">
            <Card className="border-border">
              <CardHeader className="text-center">
                <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                  <Users className="h-6 w-6 text-primary" />
                </div>
                <CardTitle className="text-2xl text-card-foreground">Контактные данные</CardTitle>
                <CardDescription>Создадим аккаунт владельца и свяжемся, если будут вопросы</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {/* Venue summary */}
                <div className="rounded-lg bg-secondary/50 p-3">
                  <div className="flex items-center gap-2">
                    <Building2 className="h-4 w-4 text-primary" />
                    <span className="font-medium text-card-foreground">{venueName}</span>
                    <Badge variant="secondary" className="text-xs">
                      {venueType === "banya" ? "Баня" : venueType === "sauna" ? "Сауна" : "Хаммам"}
                    </Badge>
                  </div>
                  <p className="mt-1 text-sm text-muted-foreground">{venueCity}, {venueAddress}</p>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="contactName">Ваше имя</Label>
                  <Input
                    id="contactName"
                    placeholder="Иван Иванов"
                    value={contactName}
                    onChange={(e) => setContactName(e.target.value)}
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="contactEmail">Email</Label>
                  <Input
                    id="contactEmail"
                    type="email"
                    placeholder="ivan@banya.ru"
                    value={contactEmail}
                    onChange={(e) => setContactEmail(e.target.value)}
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="contactPhone">Телефон</Label>
                  <PhoneInput
                    id="contactPhone"
                    value={contactPhone}
                    onChange={setContactPhone}
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="contactPassword">Придумайте пароль</Label>
                  <Input
                    id="contactPassword"
                    type="password"
                    placeholder="Минимум 8 символов"
                    value={contactPassword}
                    onChange={(e) => setContactPassword(e.target.value)}
                    required
                    minLength={8}
                  />
                </div>

                {error && <p className="text-sm text-destructive">{error}</p>}

                <div className="flex gap-3 pt-2">
                  <Button variant="outline" className="gap-2" onClick={() => setStep(2)}>
                    <ArrowLeft className="h-4 w-4" />
                    Назад
                  </Button>
                  <Button
                    className="flex-1 gap-2"
                    onClick={handleSubmit}
                    disabled={loading || !contactName || !contactEmail || !contactPhone || !contactPassword}
                  >
                    {loading ? "Создаём аккаунт и заведение..." : "Завершить регистрацию"}
                    {!loading && <CheckCircle2 className="h-4 w-4" />}
                  </Button>
                </div>
              </CardContent>
            </Card>

            <p className="mt-4 text-center text-sm text-muted-foreground">
              Шаг 3 из 3 · После регистрации вы сможете заполнить страницу заведения
            </p>
          </div>
        )}

        {/* Step 4: Success */}
        {step === 4 && (
          <div className="mx-auto max-w-lg">
            <Card className="border-border">
              <CardContent className="py-10 text-center">
                <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-green-100">
                  <PartyPopper className="h-8 w-8 text-green-600" />
                </div>
                <h2 className="mb-2 text-2xl font-bold text-card-foreground">
                  Добро пожаловать в БаняГид!
                </h2>
                <p className="mb-6 text-muted-foreground">
                  Аккаунт создан, а «{venueName}» уже добавлена на платформу. Осталось заполнить детали — цены, фото, удобства.
                </p>

                <div className="mb-6 rounded-lg bg-secondary/50 p-4 text-left">
                  <div className="flex items-center gap-2 mb-2">
                    <Building2 className="h-4 w-4 text-primary" />
                    <span className="font-medium text-card-foreground">{venueName}</span>
                    <Badge variant="secondary" className="text-xs">
                      {venueType === "banya" ? "Баня" : venueType === "sauna" ? "Сауна" : "Хаммам"}
                    </Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">{venueCity}, {venueAddress}</p>
                </div>

                <div className="flex flex-col gap-3 sm:flex-row">
                  <Button asChild className="flex-1 gap-2">
                    <Link href={`/owner/venues`}>
                      <Settings className="h-4 w-4" />
                      Перейти в личный кабинет
                    </Link>
                  </Button>
                  <Button asChild variant="outline" className="flex-1 gap-2">
                    <Link href={`/venues/${createdVenueSlug}`}>
                      <ArrowRight className="h-4 w-4" />
                      Посмотреть страницу
                    </Link>
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </div>
    </div>
  )
}
