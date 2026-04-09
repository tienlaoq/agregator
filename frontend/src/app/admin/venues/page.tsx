"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useAuthStore } from "@/store/auth"
import { getAdminVenues, moderateVenue } from "@/lib/api"
import type { Venue } from "@/lib/types"
import {
  CheckCircle2,
  XCircle,
  Pause,
  Building2,
  MapPin,
  Phone,
  Clock,
  ExternalLink,
  Shield,
} from "lucide-react"

const STATUS_LABELS: Record<string, string> = {
  pending_review: "На проверке",
  active: "Активно",
  rejected: "Отклонено",
  suspended: "Приостановлено",
}

const STATUS_COLORS: Record<string, string> = {
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
  const [moderatingId, setModeratingId] = useState<string | null>(null)
  const [comment, setComment] = useState("")

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "admin")) {
      router.push("/")
    }
  }, [hydrated, token, user, router])

  const { data, isLoading } = useQuery({
    queryKey: ["admin-venues", filterStatus],
    queryFn: () => getAdminVenues({ status: filterStatus }),
    enabled: !!token && user?.role === "admin",
  })

  const mutation = useMutation({
    mutationFn: ({
      venueId,
      action,
      comment,
    }: {
      venueId: string
      action: "approve" | "reject" | "suspend"
      comment: string
    }) => moderateVenue(venueId, action, comment),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-venues"] })
      setModeratingId(null)
      setComment("")
    },
  })

  const handleAction = (venueId: string, action: "approve" | "reject" | "suspend") => {
    if (action === "reject" && !comment.trim()) {
      setModeratingId(venueId)
      return
    }
    mutation.mutate({ venueId, action, comment })
  }

  if (!hydrated || !token) return null

  const venues: Venue[] = data?.venues ?? []

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Модерация заведений</h1>
        <Select value={filterStatus} onValueChange={setFilterStatus}>
          <SelectTrigger className="w-[200px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="pending_review">На проверке</SelectItem>
            <SelectItem value="active">Активные</SelectItem>
            <SelectItem value="rejected">Отклонённые</SelectItem>
            <SelectItem value="suspended">Приостановленные</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {!isLoading && venues.length === 0 && (
        <Card>
          <CardContent className="py-12 text-center">
            <Building2 className="mx-auto mb-4 h-12 w-12 text-muted-foreground/50" />
            <p className="text-muted-foreground">
              {filterStatus === "pending_review"
                ? "Нет заявок на модерацию"
                : `Нет заведений со статусом «${STATUS_LABELS[filterStatus]}»`}
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

              {moderatingId === venue.id && (
                <div className="space-y-2">
                  <Textarea
                    placeholder="Укажите причину отклонения..."
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    rows={2}
                  />
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="destructive"
                      disabled={!comment.trim() || mutation.isPending}
                      onClick={() => mutation.mutate({ venueId: venue.id, action: "reject", comment })}
                    >
                      Подтвердить отклонение
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setModeratingId(null)
                        setComment("")
                      }}
                    >
                      Отмена
                    </Button>
                  </div>
                </div>
              )}

              {moderatingId !== venue.id && (
                <div className="flex gap-2">
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
                  {venue.status !== "rejected" && (
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
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      {data && data.total > (data.page_size || 20) && (
        <p className="mt-6 text-center text-sm text-muted-foreground">
          Показано {venues.length} из {data.total}
        </p>
      )}
    </div>
  )
}
