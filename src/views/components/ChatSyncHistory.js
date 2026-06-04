import FormRecipient from "./generic/FormRecipient.js";

export default {
    name: 'ChatSyncHistory',
    components: {
        FormRecipient
    },
    data() {
        return {
            type: window.TYPEUSER,
            phone: '',
            count: 50,
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
            const isCountValid = this.count >= 1 && this.count <= 100;
            return isPhoneValid && isCountValid;
        },
        openModal() {
            $('#modalChatSyncHistory').modal({
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
                $('#modalChatSyncHistory').modal('hide');
            } catch (err) {
                showErrorInfo(err);
            }
        },
        async submitApi() {
            this.loading = true;
            try {
                const payload = {
                    count: Number(this.count)
                };

                const response = await window.http.post(`/chat/${this.phone_id}/sync`, payload);
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
            this.count = 50;
        },
    },
    template: `
    <div class="purple card" @click="openModal()" style="cursor: pointer">
        <div class="content">
            <a class="ui purple right ribbon label">Chat</a>
            <div class="header">Sync Old Messages</div>
            <div class="description">
                Fetch older messages for a chat on demand
            </div>
        </div>
    </div>

    <!--  Modal ChatSyncHistory  -->
    <div class="ui small modal" id="modalChatSyncHistory">
        <i class="close icon"></i>
        <div class="header">
            Sync Old Messages
        </div>
        <div class="content">
            <form class="ui form">
                <FormRecipient v-model:type="type" v-model:phone="phone" :show-status="false"/>
                <div class="field">
                    <label>Message count</label>
                    <input type="number" min="1" max="100" v-model.number="count"
                           aria-label="count" placeholder="50">
                    <small>How many messages to request before the oldest one you already have (1-100, default 50).</small>
                </div>
                <div class="ui info message">
                    Requires at least one stored message in the chat to anchor the request.
                    Retrieved messages are stored asynchronously and appear in the chat shortly after.
                </div>
            </form>
        </div>
        <div class="actions">
            <button class="ui approve positive right labeled icon button"
                 :class="{'disabled': !isValidForm() || loading, 'loading': loading}"
                 @click.prevent="handleSubmit">
                Sync Messages
                <i class="history icon"></i>
            </button>
        </div>
    </div>
    `
}
