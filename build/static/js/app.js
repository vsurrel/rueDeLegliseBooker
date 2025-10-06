'use strict';

(function () {
    const CONFIG = window.APP_CONFIG || {};
    const BASE_PATH = normaliseBasePath(CONFIG.basePath || '');
    const PEOPLE_CONFIG = Array.isArray(CONFIG.people) ? CONFIG.people : [];

    const HALF_DAY_MS = 12 * 60 * 60 * 1000;
    const MONTH_COUNT = 18;
    const WEEKDAY_LABELS = ['Lun', 'Mar', 'Mer', 'Jeu', 'Ven', 'Sam', 'Dim'];
    const MONTH_NAMES = [
        'Janvier',
        'Fevrier',
        'Mars',
        'Avril',
        'Mai',
        'Juin',
        'Juillet',
        'Aout',
        'Septembre',
        'Octobre',
        'Novembre',
        'Decembre',
    ];

    const elements = {};
    const state = {
        people: [],
        peopleMap: new Map(),
        calendarStart: null,
        slotElements: new Map(),
        indexToSlotKey: [],
        reservations: [],
        pendingRange: null,
        pendingDeleteId: null,
    };

    const selectionState = {
        anchor: null,
        anchorIndex: null,
        preview: null,
        previewIndex: null,
        pointerDown: false,
        dragging: false,
        pendingSecondClick: false,
        justFinalised: false,
    };

    let toastTimer = null;

    document.addEventListener('DOMContentLoaded', init);

    function init() {
        elements.calendar = document.getElementById('calendar');
        elements.legend = document.getElementById('legend-entries');
        elements.createModal = document.getElementById('create-modal');
        elements.createRange = document.getElementById('create-range');
        elements.personSelect = document.getElementById('person-select');
        elements.createConfirm = document.getElementById('create-confirm');
        elements.createCancel = document.getElementById('create-cancel');
        elements.createComment = document.getElementById('create-comment');
        elements.deleteModal = document.getElementById('delete-modal');
        elements.deleteDescription = document.getElementById('delete-description');
        elements.deleteConfirm = document.getElementById('delete-confirm');
        elements.deleteCancel = document.getElementById('delete-cancel');
        elements.deleteComment = document.getElementById('delete-comment');
        elements.deleteSave = document.getElementById('delete-save');
        elements.confirmModal = document.getElementById('confirm-modal');
        elements.confirmMessage = document.getElementById('confirm-message');
        elements.confirmBack = document.getElementById('confirm-back');
        elements.confirmDelete = document.getElementById('confirm-delete');
        elements.toast = document.getElementById('toast');

        state.people = PEOPLE_CONFIG;
        state.peopleMap = new Map(state.people.map((person) => [person.name, person.color]));

        initLegend();
        initPersonSelect();
        buildCalendar();
        attachGlobalListeners();
        refreshPersonSelectColor();

        loadReservations();
    }

    function initLegend() {
        elements.legend.innerHTML = '';
        state.people.forEach((person) => {
            const entry = document.createElement('div');
            entry.className = 'legend-entry';

            const swatch = document.createElement('span');
            swatch.className = 'legend-swatch';
            swatch.style.backgroundColor = person.color;

            const label = document.createElement('span');
            label.textContent = person.name;

            entry.appendChild(swatch);
            entry.appendChild(label);
            elements.legend.appendChild(entry);
        });
    }

    function initPersonSelect() {
        elements.personSelect.innerHTML = '';
        state.people.forEach((person) => {
            const option = document.createElement('option');
            option.value = person.name;
            option.textContent = person.name;
            option.dataset.color = person.color;
            elements.personSelect.appendChild(option);
        });

        if (state.people.length > 0) {
            elements.personSelect.value = state.people[0].name;
        }

        elements.personSelect.addEventListener('change', refreshPersonSelectColor);
    }

    function refreshPersonSelectColor() {
        const select = elements.personSelect;
        const color = state.peopleMap.get(select.value) || '#cccccc';
        select.style.setProperty('--person-color', color);
    }

    function buildCalendar() {
        elements.calendar.innerHTML = '';

        const start = new Date();
        start.setDate(1);
        start.setHours(0, 0, 0, 0);
        state.calendarStart = start;
        state.slotElements = new Map();
        state.indexToSlotKey = [];

        for (let monthOffset = 0; monthOffset < MONTH_COUNT; monthOffset += 1) {
            const monthDate = new Date(start.getFullYear(), start.getMonth() + monthOffset, 1);
            const monthElement = createMonthElement(monthDate);
            elements.calendar.appendChild(monthElement);
        }
    }

    function createMonthElement(monthDate) {
        const monthWrapper = document.createElement('section');
        monthWrapper.className = 'month';

        const header = document.createElement('header');
        header.className = 'month-header';
        header.textContent = `${MONTH_NAMES[monthDate.getMonth()]} ${monthDate.getFullYear()}`;
        monthWrapper.appendChild(header);

        const weekdayRow = document.createElement('div');
        weekdayRow.className = 'weekdays';
        WEEKDAY_LABELS.forEach((label) => {
            const dayLabel = document.createElement('div');
            dayLabel.textContent = label;
            weekdayRow.appendChild(dayLabel);
        });
        monthWrapper.appendChild(weekdayRow);

        const grid = document.createElement('div');
        grid.className = 'month-grid';

        const daysInMonth = new Date(monthDate.getFullYear(), monthDate.getMonth() + 1, 0).getDate();
        const leadingBlanks = (monthDate.getDay() + 6) % 7; // convert Sunday=0 to Monday=0

        for (let i = 0; i < leadingBlanks; i += 1) {
            grid.appendChild(createEmptyCell());
        }

        const today = new Date();
        today.setHours(0, 0, 0, 0);

        for (let day = 1; day <= daysInMonth; day += 1) {
            const currentDate = new Date(monthDate.getFullYear(), monthDate.getMonth(), day);
            const cell = createDayCell(currentDate);
            if (isEqualDate(currentDate, today)) {
                cell.classList.add('today');
            }
            grid.appendChild(cell);
        }

        const totalCells = leadingBlanks + daysInMonth;
        const trailingBlanks = (7 - (totalCells % 7)) % 7;
        for (let i = 0; i < trailingBlanks; i += 1) {
            grid.appendChild(createEmptyCell());
        }

        monthWrapper.appendChild(grid);
        return monthWrapper;
    }

    function createEmptyCell() {
        const cell = document.createElement('div');
        cell.className = 'day-cell empty';
        return cell;
    }

    function createDayCell(date) {
        const cell = document.createElement('div');
        cell.className = 'day-cell';
        cell.dataset.date = formatDateKey(date);

        const dayNumber = document.createElement('div');
        dayNumber.className = 'day-number';
        dayNumber.textContent = String(date.getDate());
        cell.appendChild(dayNumber);

        const slotsWrapper = document.createElement('div');
        slotsWrapper.className = 'half-slots';

        ['AM', 'PM'].forEach((half) => {
            const slot = document.createElement('div');
            slot.className = `half-slot half-${half.toLowerCase()}`;
            const slotKey = `${formatDateKey(date)}_${half}`;
            const slotIndex = state.indexToSlotKey.length;

            slot.dataset.slotKey = slotKey;
            slot.dataset.index = String(slotIndex);

            state.slotElements.set(slotKey, slot);
            state.indexToSlotKey.push(slotKey);

            slot.addEventListener('mousedown', handleSlotMouseDown);
            slot.addEventListener('mouseenter', handleSlotMouseEnter);
            slot.addEventListener('mouseup', handleSlotMouseUp);
            slot.addEventListener('click', handleSlotClick);

            slotsWrapper.appendChild(slot);
        });

        cell.appendChild(slotsWrapper);
        return cell;
    }

    function attachGlobalListeners() {
        elements.createConfirm.addEventListener('click', submitReservation);
        elements.createCancel.addEventListener('click', () => {
            closeCreateModal();
        });
        elements.deleteSave.addEventListener('click', saveReservationComment);
        elements.deleteConfirm.addEventListener('click', openConfirmModal);
        elements.deleteCancel.addEventListener('click', () => {
            closeDeleteModal();
        });
        elements.confirmBack.addEventListener('click', closeConfirmModal);
        elements.confirmDelete.addEventListener('click', confirmDeletion);

        elements.createModal.addEventListener('click', (event) => {
            if (event.target === elements.createModal) {
                closeCreateModal();
            }
        });

        elements.deleteModal.addEventListener('click', (event) => {
            if (event.target === elements.deleteModal) {
                closeDeleteModal();
            }
        });

        elements.confirmModal.addEventListener('click', (event) => {
            if (event.target === elements.confirmModal) {
                closeConfirmModal();
            }
        });

        document.addEventListener('keydown', handleKeyDown);
        document.addEventListener('mouseup', handleGlobalMouseUp);
    }

    function handleKeyDown(event) {
        if (event.key === 'Escape') {
            if (!elements.confirmModal.classList.contains('hidden')) {
                closeConfirmModal();
                return;
            }
            if (!elements.deleteModal.classList.contains('hidden')) {
                closeDeleteModal();
                return;
            }
            if (!elements.createModal.classList.contains('hidden')) {
                closeCreateModal();
                return;
            }
            if (selectionState.anchor !== null) {
                resetSelection();
            }
        }
    }

    function handleGlobalMouseUp() {
        if (!selectionState.anchor) {
            selectionState.pointerDown = false;
            return;
        }

        if (!selectionState.pointerDown) {
            return;
        }

        selectionState.pointerDown = false;

        if (selectionState.dragging && !selectionState.pendingSecondClick) {
            finalizeSelection();
            selectionState.justFinalised = true;
        }
    }

    function handleSlotMouseDown(event) {
        event.preventDefault();
        const slot = event.currentTarget;
        const slotKey = slot.dataset.slotKey;
        const index = Number(slot.dataset.index);

        if (!selectionState.anchor || !selectionState.pendingSecondClick) {
            startSelection(slotKey, index);
            return;
        }

        // Waiting for second click but user pressed mouse again: just update preview
        selectionState.pointerDown = true;
        updateSelectionPreview(slotKey, index, true);
    }

    function handleSlotMouseEnter(event) {
        const slot = event.currentTarget;
        const slotKey = slot.dataset.slotKey;
        const index = Number(slot.dataset.index);

        if (!selectionState.anchor) {
            return;
        }

        if (selectionState.pointerDown || selectionState.pendingSecondClick) {
            updateSelectionPreview(slotKey, index, selectionState.pointerDown);
        }
    }

    function handleSlotMouseUp(event) {
        if (!selectionState.anchor) {
            return;
        }

        const slot = event.currentTarget;
        const slotKey = slot.dataset.slotKey;
        const index = Number(slot.dataset.index);

        selectionState.pointerDown = false;

        if (selectionState.pendingSecondClick) {
            updateSelectionPreview(slotKey, index, false);
            return;
        }

        if (selectionState.dragging) {
            updateSelectionPreview(slotKey, index, false);
            finalizeSelection();
            selectionState.justFinalised = true;
        }
    }

    function handleSlotClick(event) {
        const slot = event.currentTarget;
        const slotKey = slot.dataset.slotKey;
        const index = Number(slot.dataset.index);

        if (selectionState.justFinalised) {
            selectionState.justFinalised = false;
            return;
        }

        if (!selectionState.anchor) {
            startSelection(slotKey, index);
            selectionState.pointerDown = false;
            selectionState.pendingSecondClick = true;
            return;
        }

        if (!selectionState.pendingSecondClick) {
            selectionState.pendingSecondClick = true;
            updateSelectionPreview(slotKey, index, false);
            return;
        }

        updateSelectionPreview(slotKey, index, false);
        finalizeSelection();
    }

    function startSelection(slotKey, index) {
        resetSelection();

        selectionState.anchor = slotKey;
        selectionState.anchorIndex = index;
        selectionState.preview = slotKey;
        selectionState.previewIndex = index;
        selectionState.pointerDown = true;
        selectionState.dragging = false;
        selectionState.pendingSecondClick = false;
        selectionState.justFinalised = false;

        applySelectionHighlight(index, index);
    }

    function updateSelectionPreview(slotKey, index, fromPointer) {
        selectionState.preview = slotKey;
        selectionState.previewIndex = index;
        if (fromPointer && index !== selectionState.anchorIndex) {
            selectionState.dragging = true;
        }
        const start = Math.min(selectionState.anchorIndex, selectionState.previewIndex);
        const end = Math.max(selectionState.anchorIndex, selectionState.previewIndex);
        applySelectionHighlight(start, end);
    }

    function applySelectionHighlight(startIndex, endIndex) {
        state.slotElements.forEach((slot) => {
            slot.classList.remove('selecting', 'selecting-start', 'selecting-end');
        });

        for (let idx = startIndex; idx <= endIndex; idx += 1) {
            const slotKey = state.indexToSlotKey[idx];
            const slotElement = state.slotElements.get(slotKey);
            if (!slotElement) {
                continue;
            }
            slotElement.classList.add('selecting');
            if (idx === startIndex) {
                slotElement.classList.add('selecting-start');
            }
            if (idx === endIndex) {
                slotElement.classList.add('selecting-end');
            }
        }
    }

    function resetSelection() {
        state.slotElements.forEach((slot) => {
            slot.classList.remove('selecting', 'selecting-start', 'selecting-end');
        });

        selectionState.anchor = null;
        selectionState.anchorIndex = null;
        selectionState.preview = null;
        selectionState.previewIndex = null;
        selectionState.pointerDown = false;
        selectionState.dragging = false;
        selectionState.pendingSecondClick = false;
        selectionState.justFinalised = false;
    }

    function finalizeSelection() {
        if (
            selectionState.anchorIndex === null ||
            selectionState.previewIndex === null ||
            selectionState.anchor === null ||
            selectionState.preview === null
        ) {
            return;
        }

        const startIndex = Math.min(selectionState.anchorIndex, selectionState.previewIndex);
        const endIndex = Math.max(selectionState.anchorIndex, selectionState.previewIndex);

        const range = {
            startIndex,
            endIndex,
            startSlotKey: state.indexToSlotKey[startIndex],
            endSlotKey: state.indexToSlotKey[endIndex],
            startDate: indexToDate(startIndex),
            endDateExclusive: indexToDate(endIndex + 1),
            halfDays: endIndex - startIndex + 1,
        };

        openCreateModal(range);
        resetSelection();
    }

    function indexToDate(index) {
        return new Date(state.calendarStart.getTime() + index * HALF_DAY_MS);
    }

    async function loadReservations() {
        try {
            const response = await fetch(buildURL('/api/reservations'));
            if (!response.ok) {
                throw new Error('fetch failed');
            }
            const data = await response.json();
            state.reservations = (Array.isArray(data) ? data : []).map((item) => ({
                id: item.id,
                person: item.person,
                start: new Date(item.start),
                end: new Date(item.end),
                comment: typeof item.comment === 'string' ? item.comment : '',
            }));
            renderReservations();
        } catch (error) {
            showToast("Impossible de charger les reservations");
        }
    }

    function renderReservations() {
        state.slotElements.forEach((slot) => {
            slot.classList.remove('has-reservation');
            const dots = slot.querySelectorAll('.reservation-dot');
            dots.forEach((dot) => dot.remove());
        });

        const sorted = state.reservations.slice().sort((a, b) => a.start - b.start);

        sorted.forEach((reservation) => {
            const slots = listSlotsForReservation(reservation);
            const color = state.peopleMap.get(reservation.person) || '#999999';
            const tooltip = reservationTooltip(reservation);

            slots.forEach((slotIndex) => {
                const slotKey = state.indexToSlotKey[slotIndex];
                const slotElement = state.slotElements.get(slotKey);
                if (!slotElement) {
                    return;
                }

                const dot = document.createElement('button');
                dot.type = 'button';
                dot.className = 'reservation-dot';
                dot.dataset.reservationId = reservation.id;
                dot.style.backgroundColor = color;
                dot.title = tooltip;
                dot.setAttribute('aria-label', tooltip);

                dot.addEventListener('mousedown', (event) => event.stopPropagation());
                dot.addEventListener('click', (event) => {
                    event.stopPropagation();
                    openDeleteModal(reservation.id);
                });

                slotElement.classList.add('has-reservation');
                slotElement.appendChild(dot);
            });
        });
    }

    function listSlotsForReservation(reservation) {
        const slots = [];
        const startIndex = dateToSlotIndex(reservation.start);
        const endIndex = dateToSlotIndex(reservation.end);
        for (let idx = startIndex; idx < endIndex; idx += 1) {
            if (idx >= 0 && idx < state.indexToSlotKey.length) {
                slots.push(idx);
            }
        }
        return slots;
    }

    function dateToSlotIndex(date) {
        const diff = date.getTime() - state.calendarStart.getTime();
        return Math.round(diff / HALF_DAY_MS);
    }

    function openCreateModal(range) {
        state.pendingRange = range;
        const startLabel = slotKeyToLabel(range.startSlotKey);
        const endLabel = slotKeyToLabel(range.endSlotKey);
        const count = range.halfDays;
        const summary =
            count === 1
                ? `1 demi-journee le ${startLabel}`
                : `${count} demi-journees du ${startLabel} au ${endLabel}`;
        elements.createRange.textContent = summary;
        if (elements.createComment) {
            elements.createComment.value = '';
        }
        elements.createModal.classList.remove('hidden');
        refreshPersonSelectColor();
        elements.personSelect.focus();
    }

    function closeCreateModal() {
        state.pendingRange = null;
        if (elements.createComment) {
            elements.createComment.value = '';
        }
        if (!elements.createModal.classList.contains('hidden')) {
            elements.createModal.classList.add('hidden');
        }
    }

    async function submitReservation() {
        const range = state.pendingRange;
        if (!range) {
            return;
        }

        const person = elements.personSelect.value;
        if (!person) {
            showToast("Merci de choisir une personne");
            return;
        }

        const comment = elements.createComment ? elements.createComment.value.trim() : '';

        const payload = {
            person,
            start: range.startDate.toISOString(),
            end: range.endDateExclusive.toISOString(),
            comment,
        };

        try {
            const response = await fetch(buildURL('/api/reservations'), {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(payload),
            });

            if (!response.ok) {
                throw new Error(await response.text());
            }

            const created = await response.json();
            state.reservations.push({
                id: created.id,
                person: created.person,
                start: new Date(created.start),
                end: new Date(created.end),
                comment: typeof created.comment === 'string' ? created.comment : comment,
            });
            closeCreateModal();
            renderReservations();
            showToast("Reservation enregistree");
        } catch (error) {
            showToast("Echec de l'enregistrement");
        }
    }

    function openDeleteModal(reservationId) {
        const reservation = state.reservations.find((item) => item.id === reservationId);
        if (!reservation) {
            return;
        }

        state.pendingDeleteId = reservationId;
        elements.deleteDescription.textContent = formatReservationSummary(reservation);
        if (elements.deleteComment) {
            elements.deleteComment.value = reservation.comment || '';
        }
        closeConfirmModal();
        elements.deleteModal.classList.remove('hidden');
    }

    function closeDeleteModal() {
        state.pendingDeleteId = null;
        if (elements.deleteComment) {
            elements.deleteComment.value = '';
        }
        if (!elements.deleteModal.classList.contains('hidden')) {
            elements.deleteModal.classList.add('hidden');
        }
        closeConfirmModal();
    }

    async function confirmDeletion() {
        if (!state.pendingDeleteId) {
            return;
        }

        const id = state.pendingDeleteId;
        try {
            const response = await fetch(buildURL(`/api/reservations/${id}`), {
                method: 'DELETE',
            });
            if (!response.ok) {
                throw new Error('delete failed');
            }

            state.reservations = state.reservations.filter((reservation) => reservation.id !== id);
            closeConfirmModal();
            closeDeleteModal();
            renderReservations();
            showToast('Reservation supprimee');
        } catch (error) {
            showToast("Echec de la suppression");
        }
    }

    function openConfirmModal() {
        if (!state.pendingDeleteId) {
            return;
        }
        if (elements.confirmMessage) {
            elements.confirmMessage.textContent = elements.deleteDescription ? elements.deleteDescription.textContent : '';
        }
        elements.confirmModal.classList.remove('hidden');
    }

    function closeConfirmModal() {
        if (!elements.confirmModal.classList.contains('hidden')) {
            elements.confirmModal.classList.add('hidden');
        }
    }

    async function saveReservationComment() {
        if (!state.pendingDeleteId) {
            return;
        }

        const id = state.pendingDeleteId;
        const comment = elements.deleteComment ? elements.deleteComment.value.trim() : '';

        try {
            const response = await fetch(buildURL(`/api/reservations/${id}`), {
                method: 'PATCH',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ comment }),
            });

            if (!response.ok) {
                throw new Error('update failed');
            }

            const updated = await response.json();
            const target = state.reservations.find((reservation) => reservation.id === id);
            if (target) {
                target.comment = typeof updated.comment === 'string' ? updated.comment : comment;
            }

            renderReservations();
            showToast('Commentaire mis a jour');
        } catch (error) {
            showToast("Echec de la mise a jour");
        }
    }

    function slotKeyToLabel(slotKey) {
        const [datePart, half] = slotKey.split('_');
        const [year, month, day] = datePart.split('-').map((value) => Number(value));
        const date = new Date(year, month - 1, day);
        const halfLabel = half === 'AM' ? 'matin' : 'apres-midi';
        return `${formatDateDisplay(date)} ${halfLabel}`;
    }

    function reservationTooltip(reservation) {
        const summary = formatReservationSummary(reservation);
        const comment = (reservation.comment || '').trim();
        return comment ? `${summary}\n${comment}` : summary;
    }

    function formatReservationSummary(reservation) {
        const startLabel = describeSlot(reservation.start);
        const endSlotDate = new Date(reservation.end.getTime() - HALF_DAY_MS);
        const endLabel = describeSlot(endSlotDate);
        const halfDays = Math.round((reservation.end - reservation.start) / HALF_DAY_MS);

        if (halfDays <= 1) {
            return `${reservation.person} - ${startLabel}`;
        }
        return `${reservation.person} - du ${startLabel} au ${endLabel}`;
    }

    function describeSlot(date) {
        const half = date.getHours() < 12 ? 'AM' : 'PM';
        const halfLabel = half === 'AM' ? 'matin' : 'apres-midi';
        return `${formatDateDisplay(date)} ${halfLabel}`;
    }

    function formatDateDisplay(date) {
        const day = String(date.getDate()).padStart(2, '0');
        const month = String(date.getMonth() + 1).padStart(2, '0');
        const year = date.getFullYear();
        return `${day}/${month}/${year}`;
    }

    function formatDateKey(date) {
        const month = String(date.getMonth() + 1).padStart(2, '0');
        const day = String(date.getDate()).padStart(2, '0');
        return `${date.getFullYear()}-${month}-${day}`;
    }

    function showToast(message) {
        window.clearTimeout(toastTimer);
        elements.toast.textContent = message;
        elements.toast.classList.remove('hidden');
        toastTimer = window.setTimeout(() => {
            elements.toast.classList.add('hidden');
        }, 3000);
    }

    function normaliseBasePath(path) {
        if (typeof path !== 'string') {
            return '';
        }
        if (path === '' || path === '/') {
            return '';
        }
        return path.endsWith('/') ? path.slice(0, -1) : path;
    }

    function buildURL(path) {
        const target = path.startsWith('/') ? path : `/${path}`;
        if (!BASE_PATH) {
            return target;
        }
        return `${BASE_PATH}${target}`;
    }

    function isEqualDate(a, b) {
        return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
    }
})();
