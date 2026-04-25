"use client"

import { useState, useRef, useMemo } from "react"
import { usePathname, useRouter } from "next/navigation"
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
import {
  register,
  createVenue,
  userFromRegisterResponse,
  ApiError,
  formatApiErrorMessage,
} from "@/lib/api"
import { useAuthStore } from "@/store/auth"
import { CityCombobox } from "@/components/banya/city-combobox"
import { AddressSuggest } from "@/components/banya/address-suggest"
import { PhoneInput, getRawPhone } from "@/components/banya/phone-input"
import { DEFAULT_VENUE_SOCIAL_LINKS, type CreateVenueRequest } from "@/lib/types"
import type { PartnerCabinetRole } from "@/lib/partner-roles"
import Link from "next/link"
import {
  Flame,
  ArrowRight,
  ArrowLeft,
  Users,
  TrendingUp,
  CalendarCheck,
  ShieldCheck,
  Building2,
  FileText,
  Link2,
  CheckCircle2,
  PartyPopper,
  Settings,
} from "lucide-react"

type Step = 1 | 2 | 3 | 4

function isOwnerVerificationComplete(
  legal: string,
  innRaw: string,
  ogrnRaw: string,
  listingUrl: string,
): boolean {
  const legalTrim = legal.trim()
  if (legalTrim.length < 3) return false
  const innD = innRaw.replace(/\D/g, "")
  if (innD.length !== 10 && innD.length !== 12) return false
  const ogrnD = ogrnRaw.replace(/\D/g, "")
  if (innD.length === 10 && ogrnD.length !== 13) return false
  if (innD.length === 12 && ogrnD.length !== 15) return false
  const u = listingUrl.trim()
  try {
    const parsed = new URL(u)
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return false
    const h = parsed.hostname.toLowerCase()
    if (h === "localhost" || h.startsWith("127.")) return false
  } catch {
    return false
  }
  return true
}

export default function PartnerPage() {
  const router = useRouter()
  const pathname = usePathname()
  const partnerRole = useMemo<PartnerCabinetRole>(
    () => (pathname?.startsWith("/partner/master") ? "master" : "venue_owner"),
    [pathname],
  )
  const isMasterTrack = partnerRole === "master"

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

  const [legalEntityName, setLegalEntityName] = useState("")
  const [inn, setInn] = useState("")
  const [ogrn, setOgrn] = useState("")
  const [publicListingUrl, setPublicListingUrl] = useState("")
  const [verificationNote, setVerificationNote] = useState("")

  // Step 3: Contact data
  const [contactName, setContactName] = useState("")
  const [contactEmail, setContactEmail] = useState("")
  const [contactPhone, setContactPhone] = useState("")
  const [contactPassword, setContactPassword] = useState("")

  const [createdVenueId, setCreatedVenueId] = useState("")
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
        role: partnerRole,
      })
      localStorage.setItem("token", res.access_token)
      localStorage.setItem("refresh_token", res.refresh_token)
      const user = userFromRegisterResponse(res, {
        name: contactName,
        email: contactEmail,
        phone: rawPhone,
        role: partnerRole,
      })
      authLogin(res.access_token, res.refresh_token, user)

      if (partnerRole === "master") {
        router.push("/owner/master/profile")
        return
      }

      const venueData: CreateVenueRequest = {
        name: venueName,
        type: venueType as CreateVenueRequest["type"],
        city: venueCity,
        address: venueAddress,
        description: venueDescription,
        phone: rawPhone,
        price_from: 0,
        capacity: 0,
        amenities: [],
        halls: [],
        services: [],
        legal_entity_name: legalEntityName.trim(),
        inn: inn.replace(/\D/g, ""),
        ogrn: ogrn.replace(/\D/g, ""),
        public_listing_url: publicListingUrl.trim(),
        verification_note: verificationNote.trim() || undefined,
        social_links: { ...DEFAULT_VENUE_SOCIAL_LINKS },
        start_as_draft: true,
      }
      const venue = await createVenue(venueData)
      setCreatedVenueId(venue.id)
      setStep(4)
    } catch (err) {
      if (err instanceof ApiError) {
        setError(formatApiErrorMessage(err, "Не удалось создать аккаунт"))
      } else {
        setError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
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
            {(isMasterTrack ? [1, 3] : [1, 2, 3]).map((s, idx, arr) => (
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
                  {s < step ? <CheckCircle2 className="h-5 w-5" /> : isMasterTrack ? idx + 1 : s}
                </div>
                <span className={`hidden text-sm sm:inline ${s === step ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                  {s === 1
                    ? "Выгоды"
                    : isMasterTrack
                      ? "Контакты"
                      : s === 2
                        ? "О заведении"
                        : "Контакты"}
                </span>
                {idx < arr.length - 1 && (
                  <div
                    className={`h-px w-8 sm:w-16 ${arr[idx] < step ? "bg-primary" : "bg-border"}`}
                  />
                )}
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
                {isMasterTrack
                  ? "Профиль пар-мастера на БаняГид"
                  : "Развивайте бизнес с БаняГид"}
              </h1>
              <p className="mx-auto max-w-xl text-lg text-muted-foreground">
                {isMasterTrack
                  ? "Заполните профиль, пройдите модерацию и получайте заявки от клиентов. Пока профиль не одобрен, он не отображается в каталоге."
                  : "Присоединяйтесь к платформе, которой доверяют сотни заведений по всей России"}
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

            <div className="flex justify-center">
              <Button
                size="lg"
                className="gap-2 px-10"
                onClick={() => setStep(isMasterTrack ? 3 : 2)}
              >
                Подать заявку
                <ArrowRight className="h-4 w-4" />
              </Button>
            </div>

            <p className="mt-4 text-center text-sm text-muted-foreground">
              Регистрация бесплатная. Без скрытых платежей.
            </p>
            <p className="mt-3 text-center text-sm text-muted-foreground">
              {isMasterTrack ? (
                <>
                  Владелец сети?{" "}
                  <Link href="/partner" className="font-medium text-primary hover:underline">
                    Оформление для владельца
                  </Link>
                </>
              ) : (
                <>
                  Вы пар-мастер?{" "}
                  <Link
                    href="/partner/master"
                    className="font-medium text-primary hover:underline"
                  >
                    Подать заявку
                  </Link>
                </>
              )}
            </p>
          </div>
        )}

        {/* Step 2: Venue Information */}
        {step === 2 && !isMasterTrack && (
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

                <div className="rounded-lg border border-border bg-muted/30 p-4 space-y-3">
                  <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                    <FileText className="h-4 w-4 text-primary" />
                    Данные для проверки владельца
                  </div>
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    Чтобы отсечь фиктивные заявки, мы сверяем ИНН и ОГРН/ОГРНИП с открытыми реестрами (ЕГРЮЛ/ЕГРИП) и смотрим публичную карточку заведения на картах. Без этого модерация невозможна.
                  </p>
                  <div className="space-y-2">
                    <Label htmlFor="legalEntityName">Полное наименование ИП или организации</Label>
                    <Input
                      id="legalEntityName"
                      placeholder="Как в выписке ЕГРЮЛ / ЕГРИП"
                      value={legalEntityName}
                      onChange={(e) => setLegalEntityName(e.target.value)}
                      required
                    />
                  </div>
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label htmlFor="inn">ИНН</Label>
                      <Input
                        id="inn"
                        inputMode="numeric"
                        placeholder="10 цифр (ООО) или 12 (ИП)"
                        value={inn}
                        onChange={(e) => setInn(e.target.value)}
                        required
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="ogrn">ОГРН или ОГРНИП</Label>
                      <Input
                        id="ogrn"
                        inputMode="numeric"
                        placeholder="13 цифр (ОГРН) или 15 (ОГРНИП)"
                        value={ogrn}
                        onChange={(e) => setOgrn(e.target.value)}
                        required
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="publicListingUrl" className="flex items-center gap-1.5">
                      <Link2 className="h-3.5 w-3.5" />
                      Ссылка на карточку на картах
                    </Label>
                    <Input
                      id="publicListingUrl"
                      type="url"
                      placeholder="https://yandex.ru/maps/org/... или 2gis.ru/..."
                      value={publicListingUrl}
                      onChange={(e) => setPublicListingUrl(e.target.value)}
                      required
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="verificationNote">Комментарий для модератора (необязательно)</Label>
                    <Textarea
                      id="verificationNote"
                      placeholder="Например: торговая марка отличается от юр. названия, доступ к телефону организации..."
                      value={verificationNote}
                      onChange={(e) => setVerificationNote(e.target.value)}
                      rows={2}
                    />
                  </div>
                </div>

                <div className="flex gap-3 pt-2">
                  <Button variant="outline" className="gap-2" onClick={() => setStep(1)}>
                    <ArrowLeft className="h-4 w-4" />
                    Назад
                  </Button>
                  <Button
                    className="flex-1 gap-2"
                    onClick={() => setStep(3)}
                    disabled={
                      !venueName ||
                      !venueType ||
                      !venueCity ||
                      !isOwnerVerificationComplete(
                        legalEntityName,
                        inn,
                        ogrn,
                        publicListingUrl,
                      )
                    }
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
                <CardDescription>
                  {isMasterTrack
                    ? "Создадим аккаунт пар-мастера; затем заполните профиль в кабинете"
                    : "Создадим аккаунт владельца и свяжемся, если будут вопросы"}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {!isMasterTrack && (
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
                )}

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
                  <Button
                    variant="outline"
                    className="gap-2"
                    onClick={() => setStep(isMasterTrack ? 1 : 2)}
                  >
                    <ArrowLeft className="h-4 w-4" />
                    Назад
                  </Button>
                  <Button
                    className="flex-1 gap-2"
                    onClick={handleSubmit}
                    disabled={loading || !contactName || !contactEmail || !contactPhone || !contactPassword}
                  >
                    {loading
                      ? isMasterTrack
                        ? "Регистрация..."
                        : "Создаём аккаунт и заведение..."
                      : isMasterTrack
                        ? "Зарегистрироваться"
                        : "Завершить регистрацию"}
                    {!loading && <CheckCircle2 className="h-4 w-4" />}
                  </Button>
                </div>
              </CardContent>
            </Card>

            <p className="mt-4 text-center text-sm text-muted-foreground">
              {isMasterTrack
                ? "Шаг 2 из 2 · Далее откроется кабинет для заполнения профиля мастера"
                : "Шаг 3 из 3 · Карточка будет создана как черновик; на модерацию вы отправите её после заполнения"}
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
                  Аккаунт создан. Карточка «{venueName}» сохранена как{" "}
                  <strong>черновик</strong>: модератор её не видит, пока вы не
                  заполните витрину и не отправите заявку на проверку из
                  редактирования карточки.
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
                  <Button asChild className="flex-1 gap-2" size="lg">
                    <Link
                      href={
                        createdVenueId
                          ? `/owner/venues/${createdVenueId}/edit`
                          : "/owner/venues"
                      }
                    >
                      <Settings className="h-4 w-4" />
                      Заполнить карточку
                    </Link>
                  </Button>
                  <Button asChild variant="outline" className="flex-1 gap-2">
                    <Link href="/owner/venues">
                      <Building2 className="h-4 w-4" />
                      Все мои заведения
                    </Link>
                  </Button>
                </div>
                <p className="mt-4 text-center text-xs text-muted-foreground">
                  Публичная страница появится после публикации; в черновике она
                  недоступна по ссылке для гостей.
                </p>
              </CardContent>
            </Card>
          </div>
        )}
      </div>
    </div>
  )
}
