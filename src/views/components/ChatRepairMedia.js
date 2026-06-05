import FormRecipient from "./generic/FormRecipient.js";

export default {
    name: 'ChatRepairMedia',
    components: {
        FormRecipient
    },
    data() {
        return {
            type: window.TYPEUSER,
            phone: '',
            limit: 50,
            loading: false,
        }
    },
    computed: {
        phone_id() {
            return this.phone + this.type;
        },
    },
    methods: {
        isValidForm() {
            const isPhoneValid = this.phone.trim().length > 0;
            const isLimitValid = this.limit >= 1 && this.limit <= 500;
            return isPhoneValid && isLimitValid;
        },
        openModal() {
            $('#modalChatRepairMedia').modal({
                onApprove: function () {
                    return false;
                }
            }).modal('show');
        },
        async handleSubmit() {
            if (!this.isValidForm() || this.loading) {
                return;
            }
            try {
                const response = await this.submitApi();
                showSuccessInfo(response);
                $('#modalChatRepairMedia').modal('hide');
            } catch (err) {
                showErrorInfo(err);
            }
        },
        async submitApi() {
            this.loading = true;
            try {
                const payload = {
                    limit: Number(this.limit)
                };

                const response = await window.http.post(`/chat/${this.phone_id}/media/repair`, payload);
                this.handleReset();
                return response.data.message;
            } catch (error) {
                if (error.response?.data?.message) {
                    throw new Error(error.response.data.message);
                }
                throw error;
            } finally {
                this.loading = false;
            }
        },
        handleReset() {
            this.phone = '';
            this.limit = 50;
        },
    },
    template: `
    <div class="purple card" @click="openModal()" style="cursor: pointer">
        <div class="content">
            <a class="ui purple right ribbon label">Chat</a>
            <div class="header">Repair Media</div>
            <div class="description">
                Re-fetch broken images/videos without re-login
            </div>
        </div>
    </div>

    <!--  Modal ChatRepairMedia  -->
    <div class="ui small modal" id="modalChatRepairMedia">
        <i class="close icon"></i>
        <div class="header">
            Repair Media
        </div>
        <div class="content">
            <form class="ui form">
                <FormRecipient v-model:type="type" v-model:phone="phone" :show-status="false"/>
                <div class="field">
                    <label>Max messages to repair</label>
                    <input type="number" min="1" max="500" v-model.number="limit"
                           aria-label="limit" placeholder="50">
                    <small>Asks the phone to re-upload media for stored messages that can't be downloaded (1-500, default 50).</small>
                </div>
                <div class="ui info message">
                    Use this when images show "Failed to download". It requests fresh media links from your phone
                    (no re-login needed). Repaired media becomes downloadable shortly after the phone responds.
                </div>
            </form>
        </div>
        <div class="actions">
            <button class="ui approve positive right labeled icon button"
                 :class="{'disabled': !isValidForm() || loading, 'loading': loading}"
                 @click.prevent="handleSubmit">
                Repair Media
                <i class="redo icon"></i>
            </button>
        </div>
    </div>
    `
}
