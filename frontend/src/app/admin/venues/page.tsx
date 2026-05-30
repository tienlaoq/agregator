"use client"

import { useEffect, useState } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Textarea } from "@/components/ui/textarea"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useAuthStore } from "@/store/auth"
import { AdminVenueFullCard } from "@/components/admin/admin-venue-full-card"
import { getAdminVenues, moderateVenue } from "@/lib/api"
import type { Venue } from "@/lib/types"
import {
  CheckCircle2,
  XCircle,
  Pause,
  RotateCw,
  Building2,
  MapPin,
  Phone,
  Clock,
  ExternalLink,
  Shield,
  Search,
  ChevronLeft,
  ChevronRight,
} from "lucide-react"

const ADMIN_VENUE_PAGE_SIZE = 50

const STATUS_LABELS: Record<string, string> = {
  draft: "Черновик",
  pending_review: "На проверке",
  active: "Активно",
  rejected: "Отклонено",
  suspended: "Приостановлено",
}

const STATUS_COLORS: Record<string, string> = {
  draft: "bg-slate-100 text-slate-800",
  pending_review: "bg-amber-100 text-amber-800",
  active: "bg-green-100 text-green-800",
  rejected: "bg-red-100 text-red-800",
  suspended: "bg-gray-100 text-gray-800",
}

export default function AdminVenuesPage() {
  const router = useRouter()
  const { token, user, hydrated } = useAuthStore()
  const queryClient = useQueryClient()
  const [filterStatus, setFilterStatus] = useState("pending_review")
  const [nameSearch, setNameSearch] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [page, setPage] = useState(1)
  /** Карточка, где ждём комментарий для «Отклонить» или «Приостановить». */
  const [moderationFlow, setModerationFlow] = useState<{
    venueId: string
    action: "reject" | "suspend"
  } | null>(null)
  const [comment, setComment] = useState("")

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "admin")) {
      router.push("/")
    }
  }, [hydrated, token, user, router])

  useEffect(() => {
    const t = window.setTimeout(() => {
      setDebouncedSearch(nameSearch.trim())
    }, 400)
    return () => window.clearTimeout(t)
  }, [nameSearch])

  // Render-time reset (React 19): сбрасываем пагинацию на 1 при смене фильтров
  // без useEffect, чтобы не нарушать react-hooks/set-state-in-effect.
  const filterKey = `${filterStatus}|${debouncedSearch}`
  const [prevFilterKey, setPrevFilterKey] = useState(filterKey)
  if (filterKey !== prevFilterKey) {
    setPrevFilterKey(filterKey)
    setPage(1)
  }

  const { data, isLoading } = useQuery({
    queryKey: ["admin-venues", filterStatus, debouncedSearch, page],
    queryFn: () =>
      getAdminVenues({
        status: filterStatus,
        page,
        page_size: ADMIN_VENUE_PAGE_SIZE,
        ...(debouncedSearch ? { q: debouncedSearch } : {}),
      }),
    enabled: !!token && user?.role === "admin",
  })

  const venues: Venue[] = data?.venues ?? []
  const total = data?.total ?? 0
  const pageSize = data?.page_size ?? ADMIN_VENUE_PAGE_SIZE
  const canPrev = page > 1
  const canNext = page * pageSize < total

  const mutation = useMutation({
    mutationFn: ({
      venueId,
      action,
      comment,
    }: {
      venueId: string
      action: "approve" | "reject" | "suspend" | "resume"
      comment: string
    }) => moderateVenue(venueId, action, comment),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-venues"] })
      setModerationFlow(null)
      setComment("")
    },
  })

  const handleAction = (venueId: string, action: "approve" | "reject" | "suspend" | "resume") => {
    if (action === "resume") {
      mutation.mutate({ venueId, action: "resume", comment: "" })
      return
    }
    if ((action === "reject" || action === "suspend") && !comment.trim()) {
      setModerationFlow({ venueId, action })
      return
    }
    mutation.mutate({ venueId, action, comment })
  }

  if (!hydrated || !token) return null

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
        <h1 className="text-2xl font-bold">Модерация заведений</h1>
        <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:items-center sm:gap-2">
          <Select value={filterStatus} onValueChange={setFilterStatus}>
            <SelectTrigger className="w-full sm:w-[200px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="draft">Черновики</SelectItem>
              <SelectItem value="pending_review">На проверке</SelectItem>
              <SelectItem value="active">Активные</SelectItem>
              <SelectItem value="rejected">Отклонённые</SelectItem>
              <SelectItem value="suspended">Приостановленные</SelectItem>
            </SelectContent>
          </Select>
          <div className="relative min-w-0 flex-1 sm:max-w-xs">
            <Search
              className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
              aria-hidden
            />
            <Input
              className="pl-8"
              placeholder="Поиск по названию…"
              value={nameSearch}
              onChange={(e) => setNameSearch(e.target.value)}
              aria-label="Поиск по названию заведения"
            />
          </div>
        </div>
      </div>

      {isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {!isLoading && venues.length === 0 && (
        <Card>
          <CardContent className="py-12 text-center">
            <Building2 className="mx-auto mb-4 h-12 w-12 text-muted-foreground/50" />
            <p className="text-muted-foreground">
              {debouncedSearch
                ? `Ничего не найдено по запросу «${debouncedSearch}»`
                : filterStatus === "pending_review"
                  ? "Нет заявок на модерацию"
                  : filterStatus === "draft"
                    ? "Нет черновиков"
                    : `Нет заведений со статусом «${STATUS_LABELS[filterStatus] ?? filterStatus}»`}
            </p>
          </CardContent>
        </Card>
      )}

      <div className="space-y-4">
        {venues.map((venue) => (
          <Card key={venue.id} className="border-border">
            <CardHeader className="pb-3">
              <div className="flex items-start justify-between">
                <div>
                  <CardTitle className="text-lg">{venue.name}</CardTitle>
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                    <Badge variant="outline" className="text-xs">
                      {venue.type === "banya" ? "Баня" : venue.type === "sauna" ? "Сауна" : "Хаммам"}
                    </Badge>
                    <span className="flex items-center gap-1">
                      <MapPin className="h-3.5 w-3.5" />
                      {venue.address || venue.city}
                    </span>
                    {venue.phone && (
                      <span className="flex items-center gap-1">
                        <Phone className="h-3.5 w-3.5" />
                        {venue.phone}
                      </span>
                    )}
                    <span className="flex items-center gap-1">
                      <Clock className="h-3.5 w-3.5" />
                      {new Date(venue.created_at).toLocaleDateString("ru-RU")}
                    </span>
                  </div>
                </div>
                <Badge className={STATUS_COLORS[venue.status || "pending_review"]}>
                  {STATUS_LABELS[venue.status || "pending_review"]}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              {venue.description && (
                <p className="text-sm text-muted-foreground">{venue.description}</p>
              )}

              <AdminVenueFullCard venue={venue} />

              {(venue.legal_entity_name || venue.inn) && (
                <div className="rounded-md border border-primary/20 bg-primary/5 p-3 text-sm">
                  <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
                    <Shield className="h-4 w-4 text-primary" />
                    Проверка владельца
                  </div>
                  {venue.legal_entity_name && (
                    <p>
                      <span className="text-muted-foreground">Юр. наименование: </span>
                      {venue.legal_entity_name}
                    </p>
                  )}
                  {(venue.inn || venue.ogrn) && (
                    <p className="mt-1 font-mono text-xs">
                      {venue.inn && <>ИНН: {venue.inn}</>}
                      {venue.inn && venue.ogrn && " · "}
                      {venue.ogrn && <>ОГРН/ОГРНИП: {venue.ogrn}</>}
                    </p>
                  )}
                  {venue.public_listing_url && (
                    <a
                      href={venue.public_listing_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="mt-2 inline-flex items-center gap-1 text-primary hover:underline"
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                      Карточка на картах
                    </a>
                  )}
                  {venue.verification_note && (
                    <p className="mt-2 text-muted-foreground">
                      <span className="font-medium text-foreground">От заявителя: </span>
                      {venue.verification_note}
                    </p>
                  )}
                </div>
              )}

              {venue.moderation_comment && (
                <div className="rounded-md bg-muted p-3 text-sm">
                  <span className="font-medium">Комментарий модератора: </span>
                  {venue.moderation_comment}
                </div>
              )}

              {moderationFlow?.venueId === venue.id && (
                <div className="space-y-2">
                  <Textarea
                    placeholder={
                      moderationFlow.action === "suspend"
                        ? "Укажите причину приостановки (обязательно)…"
                        : "Укажите причину отклонения…"
                    }
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    rows={2}
                  />
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant={moderationFlow.action === "suspend" ? "default" : "destructive"}
                      disabled={!comment.trim() || mutation.isPending}
                      onClick={() =>
                        mutation.mutate({
                          venueId: venue.id,
                          action: moderationFlow.action,
                          comment,
                        })
                      }
                    >
                      {moderationFlow.action === "suspend"
                        ? "Подтвердить приостановку"
                        : "Подтвердить отклонение"}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setModerationFlow(null)
                        setComment("")
                      }}
                    >
                      Отмена
                    </Button>
                  </div>
                </div>
              )}

              {venue.status === "draft" && (
                <p className="text-sm text-muted-foreground">
                  Черновик: владелец ещё не отправил карточку на проверку. Модерация станет доступна после
                  перехода в «На проверке».
                </p>
              )}

              {moderationFlow?.venueId !== venue.id && (
                <div className="flex flex-wrap gap-2">
                  {venue.slug ? (
                    <Button size="sm" variant="outline" className="gap-1.5" asChild>
                      <Link
                        href={`/venues/${venue.slug}`}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <ExternalLink className="h-4 w-4" />
                        Просмотр карточки
                      </Link>
                    </Button>
                  ) : null}
                  {venue.status !== "draft" && (
                    <>
                      {(venue.status === "pending_review" || venue.status === "rejected") && (
                        <Button
                          size="sm"
                          className="gap-1.5 bg-green-600 hover:bg-green-700"
                          disabled={mutation.isPending}
                          onClick={() => handleAction(venue.id, "approve")}
                        >
                          <CheckCircle2 className="h-4 w-4" />
                          Одобрить
                        </Button>
                      )}
                      {venue.status === "pending_review" && (
                        <Button
                          size="sm"
                          variant="destructive"
                          className="gap-1.5"
                          disabled={mutation.isPending}
                          onClick={() => handleAction(venue.id, "reject")}
                        >
                          <XCircle className="h-4 w-4" />
                          Отклонить
                        </Button>
                      )}
                      {venue.status === "active" && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="gap-1.5"
                          disabled={mutation.isPending}
                          onClick={() => handleAction(venue.id, "suspend")}
                        >
                          <Pause className="h-4 w-4" />
                          Приостановить
                        </Button>
                      )}
                      {venue.status === "suspended" && (
                        <Button
                          size="sm"
                          className="gap-1.5 bg-green-600 hover:bg-green-700"
                          disabled={mutation.isPending}
                          onClick={() => handleAction(venue.id, "resume")}
                        >
                          <RotateCw className="h-4 w-4" />
                          Возобновить в агрегаторе
                        </Button>
                      )}
                    </>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      {data && total > 0 ? (
        <div className="mt-6 flex flex-col items-center gap-3 sm:flex-row sm:justify-center sm:gap-6">
          <p className="text-center text-sm text-muted-foreground">
            Всего: {total}
            {debouncedSearch ? ` · поиск «${debouncedSearch}»` : ""}
            {total > pageSize
              ? ` · страница ${page} из ${Math.max(1, Math.ceil(total / pageSize))}`
              : ""}
          </p>
          {total > pageSize ? (
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="gap-1"
                disabled={!canPrev || isLoading}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                <ChevronLeft className="h-4 w-4" />
                Назад
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="gap-1"
                disabled={!canNext || isLoading}
                onClick={() => setPage((p) => p + 1)}
              >
                Вперёд
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
